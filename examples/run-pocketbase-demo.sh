#!/bin/bash
# -----------------------------------------------
# run-pocketbase-demo.sh
# A demonstration script for HarmonyLite with PocketBase
# -----------------------------------------------

set -e

# Configuration Variables
DEMO_DIR="$(pwd)/harmonylite-pb-demo"
PB_DIR_1="$DEMO_DIR/pb-1"
PB_DIR_2="$DEMO_DIR/pb-2"
MIGRATIONS_DIR="$DEMO_DIR/user_pb_migrations"
NODE1_CONFIG="$DEMO_DIR/node-1-config.toml"
NODE2_CONFIG="$DEMO_DIR/node-2-config.toml"
ADMIN_EMAIL="admin@example.com"
ADMIN_PASSWORD="1234567890"
PB_PORT_1=8090
PB_PORT_2=8091
HL_PORT_1=4221
HL_PORT_2=4222
KEEP_FILES=0
DOWNLOAD_PB=1
PB_PATH=""

# Save the original directory
ORIGINAL_DIR=$(pwd)

# Display usage information
display_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo
    echo "Sets up and runs a HarmonyLite demo with PocketBase."
    echo
    echo "Options:"
    echo "  --help              Display this help message"
    echo "  --keep-files        Don't delete temporary files on exit"
    echo "  --pb-path PATH      Specify an existing PocketBase binary"
    echo "  --no-download       Don't download PocketBase if not found"
    echo
    echo "Examples:"
    echo "  $0"
    echo "  $0 --pb-path /usr/local/bin/pocketbase"
    echo "  $0 --keep-files"
}

# Parse command-line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help)
                display_usage
                exit 0
                ;;
            --keep-files)
                KEEP_FILES=1
                shift
                ;;
            --pb-path)
                PB_PATH="$2"
                shift 2
                ;;
            --no-download)
                DOWNLOAD_PB=0
                shift
                ;;
            *)
                echo "Unknown option: $1"
                display_usage
                exit 1
                ;;
        esac
    done
}

# Clean up resources
cleanup() {
    echo "Cleaning up resources..."
    
    # Kill all background processes
    if [ -n "$pb_pid_1" ]; then
        kill -9 $pb_pid_1 2>/dev/null || true
    fi
    
    if [ -n "$pb_pid_2" ]; then
        kill -9 $pb_pid_2 2>/dev/null || true
    fi
    
    if [ -n "$hl_pid_1" ]; then
        kill -9 $hl_pid_1 2>/dev/null || true
    fi
    
    if [ -n "$hl_pid_2" ]; then
        kill -9 $hl_pid_2 2>/dev/null || true
    fi
    
    # Remove temporary files if not keeping them
    if [ "$KEEP_FILES" -eq 0 ]; then
        echo "Removing demo files..."
        rm -rf "$DEMO_DIR"
    else
        echo "Keeping demo files at $DEMO_DIR"
    fi
    
    # Return to original directory
    cd "$ORIGINAL_DIR"
    
    echo "Cleanup complete"
}

# Set up directory structure
setup_directories() {
    echo "Setting up directory structure..."
    mkdir -p "$PB_DIR_1" "$PB_DIR_2" "$MIGRATIONS_DIR"
}

# Download PocketBase if needed
download_pocketbase() {
    # If a path was specified, use it
    if [ -n "$PB_PATH" ]; then
        if [ ! -x "$PB_PATH" ]; then
            echo "Error: PocketBase not found at $PB_PATH or is not executable"
            exit 1
        fi
        PB_BIN="$PB_PATH"
        return
    fi
    
    # Check if pocketbase is in PATH
    if command -v pocketbase >/dev/null 2>&1; then
        PB_BIN="pocketbase"
        return
    fi
    
    # If download is disabled, exit
    if [ "$DOWNLOAD_PB" -eq 0 ]; then
        echo "Error: PocketBase not found and downloads are disabled"
        echo "Please install PocketBase or use --pb-path to specify its location"
        exit 1
    fi
    
    # Determine platform and architecture
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    
    case "$ARCH" in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            echo "Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac
    
    PB_VERSION="0.26.2"  # Use the latest stable version
    PB_URL="https://github.com/pocketbase/pocketbase/releases/download/v${PB_VERSION}/pocketbase_${PB_VERSION}_${OS}_${ARCH}.zip"
    PB_ZIP="$DEMO_DIR/pocketbase.zip"
    PB_BIN="$DEMO_DIR/pocketbase"
    
    echo "Downloading PocketBase v${PB_VERSION} for ${OS}_${ARCH}..."
    curl -L "$PB_URL" -o "$PB_ZIP"
    
    echo "Extracting PocketBase..."
    unzip -q "$PB_ZIP" -d "$DEMO_DIR"
    rm "$PB_ZIP"
    
    chmod +x "$PB_BIN"
    echo "PocketBase downloaded to $PB_BIN"
}

# Create PocketBase migration for notes collection
create_pb_migration() {
    echo "Creating migration file for notes collection..."
    cat > "$MIGRATIONS_DIR/init_notes.js" << 'EOF'
migrate((app) => {
   // Create a new "notes" base collection
   const collection = new Collection({
      name: "notes",
      type: "base",
      listRule: "",
      viewRule: "",
      createRule: "",
      updateRule: "",
      deleteRule: "",
      fields: [
         { name: "title", type: "text", required: true },
         { name: "content", type: "text" },
         { name: "is_important", type: "bool" }
      ]
   });
   
   return app.save(collection);
}, (app) => {
   // Down migration code (for reverting the changes)
   const collection = app.findCollectionByNameOrId("notes");
   return app.delete(collection);
});
EOF
}

# Configure PocketBase instances
configure_pocketbase() {
    echo "Configuring PocketBase instances..."
    
    # Create admin user for Node 1
    echo "Creating admin user for Node 1..."
    "$PB_BIN" superuser create "$ADMIN_EMAIL" "$ADMIN_PASSWORD" --dir="$PB_DIR_1"
    
    # Apply migrations to Node 1
    echo "Applying migrations to Node 1..."
    "$PB_BIN" migrate --dir="$PB_DIR_1" --migrationsDir="$MIGRATIONS_DIR"
    
    # Clone configuration to Node 2
    echo "Cloning configuration to Node 2..."
    cp -r "$PB_DIR_1"/* "$PB_DIR_2"/
}

# Create HarmonyLite configuration files
create_harmonylite_configs() {
    echo "Creating HarmonyLite configuration files..."
    
    # Create Node 1 config
    cat > "$NODE1_CONFIG" << EOF
db_path = "$PB_DIR_1/data.db"
node_id = 1
seq_map_path = "$PB_DIR_1/seq-map.cbor"
[replication_log]
shards = 1
max_entries = 1024
replicas = 2
compress = true
[snapshot]
enabled = true
interval = 3600000
store = "nats"
EOF
    
    # Create Node 2 config
    cat > "$NODE2_CONFIG" << EOF
db_path = "$PB_DIR_2/data.db"
node_id = 2
seq_map_path = "$PB_DIR_2/seq-map.cbor"
[replication_log]
shards = 1
max_entries = 1024
replicas = 2
compress = true
[snapshot]
enabled = true
interval = 3600000
store = "nats"
EOF
}

# Display access instructions
display_instructions() {
    echo
    echo "========================================================"
    echo "                DEMO IS NOW RUNNING                     "
    echo "========================================================"
    echo
    echo "PocketBase Admin Dashboards:"
    echo "  Node 1: http://localhost:$PB_PORT_1/_/"
    echo "  Node 2: http://localhost:$PB_PORT_2/_/"
    echo
    echo "Login credentials for both nodes:"
    echo "  Email: $ADMIN_EMAIL"
    echo "  Password: $ADMIN_PASSWORD"
    echo
    echo "To test replication:"
    echo "  1. Open Node 1's dashboard and add a note in the 'notes' collection"
    echo "  2. Check Node 2's dashboard to see the note replicated"
    echo
    echo "Press CTRL+C to stop the demo"
    echo "========================================================"
}

# Find the HarmonyLite binary
find_harmonylite() {
    # Check if harmonylite is in the current directory
    if [ -x "./harmonylite" ]; then
        HL_BIN="./harmonylite"
        return
    fi
    
    # Check if harmonylite is in PATH
    if command -v harmonylite >/dev/null 2>&1; then
        HL_BIN="harmonylite"
        return
    fi
    
    # Check one directory up (in case we're in examples/)
    if [ -x "../harmonylite" ]; then
        HL_BIN="../harmonylite"
        return
    fi
    
    echo "Error: HarmonyLite binary not found"
    echo "Please ensure the 'harmonylite' binary is in your PATH or in the current directory"
    exit 1
}

# Run the demo
run_demo() {
    # Start PocketBase instances
    echo "Starting PocketBase instances..."
    "$PB_BIN" serve --dir="$PB_DIR_1" --http="localhost:$PB_PORT_1" &
    pb_pid_1=$!
    
    "$PB_BIN" serve --dir="$PB_DIR_2" --http="localhost:$PB_PORT_2" &
    pb_pid_2=$!
    
    # Wait for PocketBase to initialize
    echo "Waiting for PocketBase to initialize..."
    sleep 3
    
    # Start HarmonyLite nodes
    echo "Starting HarmonyLite nodes..."
    "$HL_BIN" -config "$NODE1_CONFIG" -cluster-addr "localhost:$HL_PORT_1" -cluster-peers "nats://localhost:$HL_PORT_2/" &
    hl_pid_1=$!
    
    sleep 1
    
    "$HL_BIN" -config "$NODE2_CONFIG" -cluster-addr "localhost:$HL_PORT_2" -cluster-peers "nats://localhost:$HL_PORT_1/" &
    hl_pid_2=$!
    
    # Display instructions
    display_instructions
    
    # Wait for CTRL+C
    wait $pb_pid_1 $pb_pid_2 $hl_pid_1 $hl_pid_2
}

# Main execution flow
main() {
    # Parse command-line arguments
    parse_args "$@"
    
    # Set up trap for clean exit
    trap cleanup EXIT INT TERM
    
    # Set up directory structure
    setup_directories
    
    # Download PocketBase if needed
    download_pocketbase
    
    # Create PocketBase migration
    create_pb_migration
    
    # Configure PocketBase instances
    configure_pocketbase
    
    # Create HarmonyLite configuration files
    create_harmonylite_configs
    
    # Find HarmonyLite binary
    find_harmonylite
    
    # Run the demo
    run_demo
}

# Start execution
main "$@"
