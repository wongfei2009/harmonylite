package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/wongfei2009/harmonylite/telemetry"
	"github.com/wongfei2009/harmonylite/utils"
	"github.com/wongfei2009/harmonylite/version"

	"github.com/wongfei2009/harmonylite/cfg"
	"github.com/wongfei2009/harmonylite/db"
	"github.com/wongfei2009/harmonylite/health"
	"github.com/wongfei2009/harmonylite/logstream"
	"github.com/wongfei2009/harmonylite/snapshot"

	"github.com/asaskevich/EventBus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	versionFlag := flag.Bool("version", false, "Display version information")
	flag.Parse()

	if *versionFlag {
		fmt.Println(version.Get().String())
		return
	}
	err := cfg.Load(*cfg.ConfigPathFlag)
	if err != nil {
		panic(err)
	}

	var writer io.Writer = zerolog.NewConsoleWriter()
	if cfg.Config.Logging.Format == "json" {
		writer = os.Stdout
	}
	gLog := zerolog.New(writer).
		With().
		Timestamp().
		Uint64("node_id", cfg.Config.NodeID).
		Logger()

	if cfg.Config.Logging.Verbose {
		log.Logger = gLog.Level(zerolog.DebugLevel)
	} else {
		log.Logger = gLog.Level(zerolog.InfoLevel)
	}

	if *cfg.ProfServer != "" {
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/debug/pprof/", pprof.Index)
			mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

			err := http.ListenAndServe(*cfg.ProfServer, mux)
			if err != nil {
				log.Error().Err(err).Msg("unable to bind profiler server")
			}
		}()
	}

	log.Debug().Msg("Initializing telemetry")
	telemetry.InitializeTelemetry()

	// Initialize default health check config if not set
	if cfg.Config.HealthCheck == nil {
		cfg.Config.HealthCheck = health.DefaultConfig()
	}

	log.Debug().Str("path", cfg.Config.DBPath).Msg("Checking if database file exists")
	if _, err := os.Stat(cfg.Config.DBPath); os.IsNotExist(err) {
		log.Error().Str("path", cfg.Config.DBPath).Msg("Database file does not exist. HarmonyLite is meant to replicate an existing database.")
		return
	}

	log.Debug().Str("path", cfg.Config.DBPath).Msg("Opening database")
	streamDB, err := db.OpenStreamDB(cfg.Config.DBPath)
	if err != nil {
		log.Error().Err(err).Msg("Unable to open database")
		return
	}

	if *cfg.CleanupFlag {
		err = streamDB.RemoveCDC(true)
		if err != nil {
			log.Panic().Err(err).Msg("Unable to clean up...")
		} else {
			log.Info().Msg("Cleanup complete...")
		}

		return
	}

	// Handle schema status commands
	if *cfg.SchemaStatusFlag || *cfg.SchemaStatusClusterFlag {
		// For cluster status, we need the replicator
		if *cfg.SchemaStatusClusterFlag {
			snpStore, err := snapshot.NewSnapshotStorage()
			if err != nil {
				log.Panic().Err(err).Msg("Unable to initialize snapshot storage")
			}

			replicator, err := logstream.NewReplicator(snapshot.NewNatsDBSnapshot(streamDB, snpStore))
			if err != nil {
				log.Panic().Err(err).Msg("Unable to initialize replicators")
			}

			printClusterSchemaStatus(replicator)
		} else {
			printLocalSchemaStatus(streamDB)
		}
		return
	}

	snpStore, err := snapshot.NewSnapshotStorage()
	if err != nil {
		log.Panic().Err(err).Msg("Unable to initialize snapshot storage")
	}

	replicator, err := logstream.NewReplicator(snapshot.NewNatsDBSnapshot(streamDB, snpStore))
	if err != nil {
		log.Panic().Err(err).Msg("Unable to initialize replicators")
	}

	// Initialize health check server
	if cfg.Config.HealthCheck.Enable {
		// streamDB implements health.DBChecker
		// replicator implements health.ReplicationChecker
		healthChecker := health.NewHealthChecker(streamDB, replicator, cfg.Config.NodeID, version.Get().Version)
		healthServer := health.NewHealthServer(cfg.Config.HealthCheck, healthChecker)

		if err := healthServer.Start(); err != nil {
			log.Warn().Err(err).Msg("Failed to start health check server")
		}

		// Make sure health server is stopped when application exits
		defer func() {
			if err := healthServer.Stop(); err != nil {
				log.Warn().Err(err).Msg("Error stopping health check server")
			}
		}()
	}

	if *cfg.SaveSnapshotFlag {
		replicator.ForceSaveSnapshot()
		return
	}

	if cfg.Config.Snapshot.Enable && cfg.Config.Replicate {
		err = replicator.RestoreSnapshot()
		if err != nil {
			log.Panic().Err(err).Msg("Unable to restore snapshot")
		}
	}

	// Start snapshot leader election for publisher nodes
	if cfg.Config.Snapshot.Enable && cfg.Config.Publish {
		replicator.StartSnapshotLeader()
		defer replicator.StopSnapshotLeader()
	}

	log.Info().Msg("Listing tables to watch...")
	tableNames, err := db.GetAllDBTables(cfg.Config.DBPath)
	if err != nil {
		log.Error().Err(err).Msg("Unable to list all tables")
		return
	}

	eventBus := EventBus.New()
	ctxSt := utils.NewStateContext()

	streamDB.OnChange = onTableChanged(replicator, ctxSt, eventBus, cfg.Config.NodeID)
	log.Info().Msg("Starting change data capture pipeline...")
	if err := streamDB.InstallCDC(tableNames); err != nil {
		log.Error().Err(err).Msg("Unable to install change data capture pipeline")
		return
	}

	// Publish schema state to registry after CDC installation
	schemaHash := streamDB.GetSchemaHash()
	if schemaHash != "" {
		if err := replicator.PublishSchemaState(schemaHash); err != nil {
			log.Warn().Err(err).Msg("Failed to publish schema state to registry")
		} else {
			hashPreview := schemaHash
			if len(schemaHash) > 16 {
				hashPreview = schemaHash[:16] + "..."
			}
			log.Info().
				Str("schema_hash", hashPreview).
				Msg("Published schema state to cluster registry")
		}
	}

	errChan := make(chan error)
	for i := uint64(0); i < cfg.Config.ReplicationLog.Shards; i++ {
		go changeListener(streamDB, replicator, ctxSt, eventBus, i+1, errChan)
	}

	sleepTimeout := utils.AutoResetEventTimer(
		eventBus,
		"pulse",
		time.Duration(cfg.Config.SleepTimeout)*time.Millisecond,
	)
	cleanupInterval := time.Duration(cfg.Config.CleanupInterval) * time.Millisecond
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	snapshotInterval := time.Duration(cfg.Config.Snapshot.Interval) * time.Millisecond
	snapshotTicker := utils.NewTimeoutPublisher(snapshotInterval)
	defer snapshotTicker.Stop()

	for {
		select {
		case err = <-errChan:
			if err != nil {
				log.Panic().Err(err).Msg("Terminated listener")
			}
		case t := <-cleanupTicker.C:
			cnt, err := streamDB.CleanupChangeLogs(t.Add(-cleanupInterval))
			if err != nil {
				log.Warn().Err(err).Msg("Unable to cleanup change logs")
			} else if cnt > 0 {
				log.Debug().Int64("count", cnt).Msg("Cleaned up DB change logs")
			}
		case <-snapshotTicker.Channel():
			if cfg.Config.Snapshot.Enable && cfg.Config.Publish {
				lastSnapshotTime := replicator.LastSaveSnapshotTime()
				now := time.Now()
				if now.Sub(lastSnapshotTime) >= snapshotInterval {
					log.Info().
						Time("last_snapshot", lastSnapshotTime).
						Dur("duration", now.Sub(lastSnapshotTime)).
						Msg("Triggering timer based snapshot save")
					replicator.SaveSnapshot()
				}
			}
		case <-sleepTimeout.Channel():
			log.Info().Msg("No more events to process, initiating shutdown")
			ctxSt.Cancel()
			if cfg.Config.Snapshot.Enable && cfg.Config.Publish {
				// Stop leader election and save final snapshot if we're the leader
				if replicator.IsSnapshotLeader() {
					log.Info().Msg("Saving snapshot before going to sleep")
					replicator.ForceSaveSnapshot()
				}
			}

			os.Exit(0)
		}
	}
}

func changeListener(
	streamDB *db.SqliteStreamDB,
	rep *logstream.Replicator,
	ctxSt *utils.StateContext,
	events EventBus.BusPublisher,
	shard uint64,
	errChan chan error,
) {
	log.Debug().Uint64("shard", shard).Msg("Listening stream")
	err := rep.ListenWithDB(shard, streamDB, onChangeEventSimple(ctxSt, events))
	if err != nil {
		errChan <- err
	}
}

func onChangeEventSimple(ctxSt *utils.StateContext, events EventBus.BusPublisher) func(event *db.ChangeLogEvent) error {
	return func(event *db.ChangeLogEvent) error {
		events.Publish("pulse")
		if ctxSt.IsCanceled() {
			return context.Canceled
		}

		if !cfg.Config.Replicate {
			return nil
		}

		// Event has already been replicated by ListenWithDB
		// This callback is for any additional processing
		return nil
	}
}

func printLocalSchemaStatus(streamDB *db.SqliteStreamDB) {
	fmt.Println("Local Schema Status")
	fmt.Println("===================")

	schemaHash := streamDB.GetSchemaHash()
	if schemaHash == "" {
		fmt.Println("Schema hash: Not initialized")
		return
	}

	fmt.Printf("Schema Hash: %s\n", schemaHash)
	fmt.Printf("Node ID: %d\n", cfg.Config.NodeID)
	fmt.Printf("HarmonyLite Version: %s\n", version.Get().Version)

	// Get watched tables
	tableNames, err := db.GetAllDBTables(cfg.Config.DBPath)
	if err != nil {
		fmt.Printf("Error listing tables: %v\n", err)
		return
	}

	fmt.Println("\nWatched Tables:")
	for _, table := range tableNames {
		fmt.Printf("  - %s\n", table)
	}
}

func printClusterSchemaStatus(replicator *logstream.Replicator) {
	fmt.Println("Cluster Schema Status")
	fmt.Println("=====================")

	states, err := replicator.GetClusterSchemaState()
	if err != nil {
		fmt.Printf("Error retrieving cluster schema state: %v\n", err)
		return
	}

	if len(states) == 0 {
		fmt.Println("No schema state information available from cluster")
		return
	}

	fmt.Printf("Total Nodes: %d\n", len(states))

	// Check consistency
	report, err := replicator.CheckClusterSchemaConsistency()
	if err != nil {
		fmt.Printf("Error checking consistency: %v\n", err)
		return
	}

	if report.Consistent {
		fmt.Println("Consistent: Yes")
	} else {
		fmt.Println("Consistent: No")
	}

	// Group nodes by hash
	hashGroups := make(map[string][]uint64)
	for nodeID, state := range states {
		hashGroups[state.SchemaHash] = append(hashGroups[state.SchemaHash], nodeID)
	}

	fmt.Println("\nHash Groups:")
	for hash, nodeIDs := range hashGroups {
		hashPreview := hash
		if len(hash) > 16 {
			hashPreview = hash[:16]
		}
		fmt.Printf("  %s: ", hashPreview)
		for i, nodeID := range nodeIDs {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("Node %d", nodeID)
		}
		fmt.Println()
	}

	// Show detailed node information
	fmt.Println("\nNode Details:")
	for nodeID, state := range states {
		fmt.Printf("  Node %d:\n", nodeID)
		fmt.Printf("    Schema Hash: %s\n", state.SchemaHash)
		fmt.Printf("    HarmonyLite Version: %s\n", state.HarmonyLiteVersion)
		fmt.Printf("    Updated At: %s\n", state.UpdatedAt.Format(time.RFC3339))
	}

	// Show mismatches if any
	if !report.Consistent {
		fmt.Println("\nSchema Mismatches:")
		for _, mismatch := range report.Mismatches {
			expPreview := mismatch.ExpectedHash
			actPreview := mismatch.ActualHash
			if len(expPreview) > 16 {
				expPreview = expPreview[:16]
			}
			if len(actPreview) > 16 {
				actPreview = actPreview[:16]
			}
			fmt.Printf("  Node %d: hash mismatch (expected: %s, actual: %s)\n",
				mismatch.NodeId, expPreview, actPreview)
		}
	}
}

func onChangeEvent(streamDB *db.SqliteStreamDB, ctxSt *utils.StateContext, events EventBus.BusPublisher) func(data []byte) error {
	return func(data []byte) error {
		events.Publish("pulse")
		if ctxSt.IsCanceled() {
			return context.Canceled
		}

		if !cfg.Config.Replicate {
			return nil
		}

		ev := &logstream.ReplicationEvent[db.ChangeLogEvent]{}
		err := ev.Unmarshal(data)
		if err != nil {
			log.Error().Err(err).Send()
			return err
		}

		return streamDB.Replicate(&ev.Payload)
	}
}

func onTableChanged(r *logstream.Replicator, ctxSt *utils.StateContext, events EventBus.BusPublisher, nodeID uint64) func(event *db.ChangeLogEvent) error {
	return func(event *db.ChangeLogEvent) error {
		events.Publish("pulse")
		if ctxSt.IsCanceled() {
			return context.Canceled
		}

		if !cfg.Config.Publish {
			return nil
		}

		ev := &logstream.ReplicationEvent[db.ChangeLogEvent]{
			FromNodeId: nodeID,
			Payload:    *event,
		}

		data, err := ev.Marshal()
		if err != nil {
			return err
		}

		hash, err := event.Hash()
		if err != nil {
			return err
		}

		err = r.Publish(hash, data)
		if err != nil {
			return err
		}

		return nil
	}
}
