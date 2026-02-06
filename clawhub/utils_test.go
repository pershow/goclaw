package clawhub

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{"valid simple", "my-skill", false},
		{"valid with numbers", "my-skill-123", false},
		{"valid with underscore", "my_skill", false},
		{"valid mixed", "my_skill-123", false},
		{"empty", "", true},
		{"uppercase", "MySkill", true},
		{"starts with hyphen", "-skill", true},
		{"ends with hyphen", "skill-", true},
		{"too short", "a", true},
		{"too long", "this-is-a-very-long-slug-name-that-exceeds-fifty-characters-limit", true},
		{"special chars", "skill@123", true},
		{"spaces", "my skill", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSlug(tt.slug)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSlug() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid simple", "1.0.0", false},
		{"valid with prerelease", "1.0.0-alpha", false},
		{"valid with build", "1.0.0+build", false},
		{"valid complex", "1.2.3-beta.1+build.123", false},
		{"missing minor", "1", false}, // go-version accepts this
		{"missing patch", "1.0", false}, // go-version accepts this
		{"invalid chars", "a.b.c", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBumpVersion(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		bumpType       string
		expected       string
		wantErr        bool
	}{
		{"patch bump", "1.0.0", "patch", "1.0.1", false},
		{"minor bump", "1.0.0", "minor", "1.1.0", false},
		{"major bump", "1.0.0", "major", "2.0.0", false},
		{"patch with existing", "1.2.3", "patch", "1.2.4", false},
		{"minor with existing", "1.2.3", "minor", "1.3.0", false},
		{"major with existing", "1.2.3", "major", "2.0.0", false},
		{"invalid bump type", "1.0.0", "invalid", "", true},
		{"invalid version", "invalid", "patch", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BumpVersion(tt.currentVersion, tt.bumpType)
			if (err != nil) != tt.wantErr {
				t.Errorf("BumpVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("BumpVersion() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
		wantErr  bool
	}{
		{"equal", "1.0.0", "1.0.0", 0, false},
		{"v1 greater", "2.0.0", "1.0.0", 1, false},
		{"v1 less", "1.0.0", "2.0.0", -1, false},
		{"minor greater", "1.1.0", "1.0.0", 1, false},
		{"patch greater", "1.0.1", "1.0.0", 1, false},
		{"invalid v1", "invalid", "1.0.0", 0, true},
		{"invalid v2", "1.0.0", "invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompareVersions(tt.v1, tt.v2)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("CompareVersions() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name       string
		basePath   string
		inputPath  string
		wantErr    bool
		checkStart string
	}{
		{
			name:       "valid relative path",
			basePath:   "/base",
			inputPath:  "subdir/file.txt",
			wantErr:    false,
			checkStart: "/base",
		},
		{
			name:       "path traversal attempt",
			basePath:   "/base",
			inputPath:  "../etc/passwd",
			wantErr:    true,
			checkStart: "",
		},
		{
			name:       "absolute path",
			basePath:   "/base",
			inputPath:  "/etc/passwd",
			wantErr:    false, // SanitizePath just joins paths
			checkStart: "/base",
		},
		{
			name:       "normal path",
			basePath:   "/base",
			inputPath:  "file.txt",
			wantErr:    false,
			checkStart: "/base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SanitizePath(tt.basePath, tt.inputPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SanitizePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != "" {
				if len(result) < len(tt.checkStart) || result[:len(tt.checkStart)] != tt.checkStart {
					t.Errorf("SanitizePath() result = %v, should start with %v", result, tt.checkStart)
				}
			}
		})
	}
}

func TestValidateSkillDir(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "clawhub-skill-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with SKILL.md
	validDir := filepath.Join(tmpDir, "valid-skill")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}

	skillFile := filepath.Join(validDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# Test Skill"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateSkillDir(validDir); err != nil {
		t.Errorf("ValidateSkillDir() error = %v", err)
	}

	// Test without SKILL.md
	invalidDir := filepath.Join(tmpDir, "invalid-skill")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := ValidateSkillDir(invalidDir); err == nil {
		t.Error("expected error for directory without SKILL.md")
	}

	// Test non-existent directory
	if err := ValidateSkillDir("/non/existent/path"); err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestCreateZipBundle(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "clawhub-zip-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	skillDir := filepath.Join(tmpDir, "skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("# Test Skill\n\nThis is a test."), 0644); err != nil {
		t.Fatal(err)
	}

	readmeFile := filepath.Join(skillDir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# README"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create hidden file (should be excluded)
	hiddenFile := filepath.Join(skillDir, ".hidden")
	if err := os.WriteFile(hiddenFile, []byte("hidden"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create bundle
	bundle, err := CreateZipBundle(skillDir)
	if err != nil {
		t.Fatalf("CreateZipBundle() error = %v", err)
	}

	// Verify bundle contents
	reader := bytes.NewReader(bundle)
	zipReader, err := zip.NewReader(reader, int64(len(bundle)))
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}

	// Check files
	foundSkill := false
	foundReadme := false
	foundHidden := false

	for _, file := range zipReader.File {
		switch file.Name {
		case "SKILL.md":
			foundSkill = true
		case "README.md":
			foundReadme = true
		case ".hidden":
			foundHidden = true
		}
	}

	if !foundSkill {
		t.Error("SKILL.md not found in bundle")
	}

	if !foundReadme {
		t.Error("README.md not found in bundle")
	}

	if foundHidden {
		t.Error(".hidden file should be excluded from bundle")
	}
}

func TestExtractZipBundle(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "clawhub-extract-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a simple zip
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	writer, err := zipWriter.Create("test.txt")
	if err != nil {
		t.Fatal(err)
	}

	writer.Write([]byte("test content"))

	zipWriter.Close()

	// Extract
	destDir := filepath.Join(tmpDir, "extracted")
	if err := ExtractZipBundle(buf.Bytes(), destDir); err != nil {
		t.Fatalf("ExtractZipBundle() error = %v", err)
	}

	// Verify extraction
	testFile := filepath.Join(destDir, "test.txt")
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}

	if string(data) != "test content" {
		t.Errorf("expected 'test content', got '%s'", string(data))
	}
}

func TestExtractZipBundlePathTraversal(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "clawhub-extract-traversal-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a zip with path traversal attempt
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Try to create a file outside the extraction directory
	writer, err := zipWriter.Create("../traversal.txt")
	if err != nil {
		t.Fatal(err)
	}

	writer.Write([]byte("malicious content"))

	zipWriter.Close()

	// Extract - should fail
	destDir := filepath.Join(tmpDir, "extracted")
	if err := ExtractZipBundle(buf.Bytes(), destDir); err == nil {
		t.Error("expected error for path traversal attempt")
	}

	// Verify file was not created outside destDir
	traversalFile := filepath.Join(tmpDir, "traversal.txt")
	if _, err := os.Stat(traversalFile); !os.IsNotExist(err) {
		t.Error("path traversal file was created outside destination")
	}
}
