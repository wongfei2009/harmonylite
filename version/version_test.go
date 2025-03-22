package version

import (
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()
	if info.Version != version {
		t.Errorf("Expected Version %s, got %s", version, info.Version)
	}
	if info.GitCommit != gitCommit {
		t.Errorf("Expected GitCommit %s, got %s", gitCommit, info.GitCommit)
	}
	if info.GitTag != gitTag {
		t.Errorf("Expected GitTag %s, got %s", gitTag, info.GitTag)
	}
	if info.BuildDate != buildDate {
		t.Errorf("Expected BuildDate %s, got %s", buildDate, info.BuildDate)
	}
	if info.GoVersion != goVersion {
		t.Errorf("Expected GoVersion %s, got %s", goVersion, info.GoVersion)
	}
	if info.Platform != platform {
		t.Errorf("Expected Platform %s, got %s", platform, info.Platform)
	}
}

func TestString(t *testing.T) {
	info := Get()
	str := info.String()
	
	// Check that all fields are in the string
	expectedParts := []string{
		"HarmonyLite", info.Version, "git:", info.GitCommit, "tag:", info.GitTag, 
		"built:", info.BuildDate, info.GoVersion, info.Platform,
	}
	
	for _, part := range expectedParts {
		if !strings.Contains(str, part) {
			t.Errorf("Expected String() to contain %s, but got: %s", part, str)
		}
	}
}

func TestShortString(t *testing.T) {
	// Set a mock commit hash for testing the 7-character truncation
	origGitCommit := gitCommit
	gitCommit = "abcdef1234567890"
	defer func() { gitCommit = origGitCommit }()
	
	info := Get()
	shortStr := info.ShortString()
	
	if !strings.Contains(shortStr, info.Version) {
		t.Errorf("Expected ShortString() to contain version %s, but got: %s", info.Version, shortStr)
	}
	
	// Check for truncated commit hash (7 chars)
	if !strings.Contains(shortStr, gitCommit[:7]) {
		t.Errorf("Expected ShortString() to contain truncated commit %s, but got: %s", gitCommit[:7], shortStr)
	}
}