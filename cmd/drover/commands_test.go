package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCmd(t *testing.T) {
	// Create a temp directory
	tempDir := t.TempDir()

	// Save original working dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// Change to temp dir
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Create and run init command
	cmd := initCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("initCmd failed: %v", err)
	}

	// Verify .drover directory exists
	droverDir := filepath.Join(tempDir, ".drover")
	if stat, err := os.Stat(droverDir); err != nil || !stat.IsDir() {
		t.Errorf(".drover directory not created")
	}

	// Verify db exists
	dbPath := filepath.Join(droverDir, "drover.db")
	if stat, err := os.Stat(dbPath); err != nil || stat.Size() == 0 {
		t.Errorf("database not created")
	}

	// Running init again should fail
	cmd2 := initCmd()
	cmd2.SetOut(&buf)
	cmd2.SetArgs([]string{})
	if err := cmd2.Execute(); err == nil {
		t.Errorf("expected error when running init again, got nil")
	}
}

func setupTestProject(t *testing.T) string {
	tempDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tempDir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cmd := initCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	return tempDir
}

func TestAddCmd(t *testing.T) {
	setupTestProject(t)

	cmd := addCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"Add a test feature", "--skip-validation", "--description", "This is a test description"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("addCmd failed: %v", err)
	}
	
	// Should fail with missing args
	cmdMissing := addCmd()
	cmdMissing.SetOut(&buf)
	cmdMissing.SetArgs([]string{})
	if err := cmdMissing.Execute(); err == nil {
		t.Errorf("expected error with missing args")
	}
}

func TestEpicCmd(t *testing.T) {
	setupTestProject(t)

	cmd := epicCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"add", "Test Epic", "--description", "Epic Description"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("epicCmd add failed: %v", err)
	}
}

func TestStatusCmd(t *testing.T) {
	setupTestProject(t)

	// Add a task so status isn't totally empty
	add := addCmd()
	add.SetArgs([]string{"Test task", "--skip-validation"})
	_ = add.Execute()

	cmd := statusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("statusCmd failed: %v", err)
	}
}

func TestTreeCmd(t *testing.T) {
	setupTestProject(t)

	cmd := statusCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--tree"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("statusCmd --tree failed: %v", err)
	}
}

func TestInfoCmd(t *testing.T) {
	setupTestProject(t)

	// info without args should fail
	cmd := infoCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for missing task id")
	}

	// info with non-existent task should fail
	cmd = infoCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"non-existent-task"})
	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for non-existent task")
	}
}

func TestResetCmd(t *testing.T) {
	setupTestProject(t)

	cmd := resetCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("resetCmd failed: %v", err)
	}
}

func TestCancelCmd(t *testing.T) {
	setupTestProject(t)

	cmd := cancelCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"non-existent-task"})

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for non-existent task")
	}
}

func TestRetryCmd(t *testing.T) {
	setupTestProject(t)

	cmd := retryCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"non-existent-task"})

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for non-existent task")
	}
}

func TestResolveCmd(t *testing.T) {
	setupTestProject(t)

	cmd := resolveCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"non-existent-task"})

	if err := cmd.Execute(); err == nil {
		t.Errorf("expected error for non-existent task")
	}
}

func TestExportCmd(t *testing.T) {
	setupTestProject(t)

	cmd := exportCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("exportCmd failed: %v", err)
	}
}
