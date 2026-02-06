package clawhub

import (
	"os"
	"testing"
	"time"
)

func TestNewLockfile(t *testing.T) {
	lf := NewLockfile()

	if lf.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", lf.Version)
	}

	if lf.Skills == nil {
		t.Error("expected skills map to be initialized")
	}

	if len(lf.Skills) != 0 {
		t.Errorf("expected empty skills map, got %d skills", len(lf.Skills))
	}
}

func TestLockfileAddSkill(t *testing.T) {
	lf := NewLockfile()

	slug := "test-skill"
	name := "Test Skill"
	version := "1.0.0"
	hash := "sha256:abc123"
	tags := []string{"latest", "test"}

	lf.AddSkill(slug, name, version, hash, tags)

	if !lf.HasSkill(slug) {
		t.Error("expected skill to be added")
	}

	skill, ok := lf.GetSkill(slug)
	if !ok {
		t.Fatal("expected skill to exist")
	}

	if skill.Name != name {
		t.Errorf("expected name %s, got %s", name, skill.Name)
	}

	if skill.Version != version {
		t.Errorf("expected version %s, got %s", version, skill.Version)
	}

	if skill.Hash != hash {
		t.Errorf("expected hash %s, got %s", hash, skill.Hash)
	}
}

func TestLockfileRemoveSkill(t *testing.T) {
	lf := NewLockfile()

	slug := "test-skill"
	lf.AddSkill(slug, "Test", "1.0.0", "sha256:abc", []string{})

	if !lf.HasSkill(slug) {
		t.Error("expected skill to exist before removal")
	}

	lf.RemoveSkill(slug)

	if lf.HasSkill(slug) {
		t.Error("expected skill to be removed")
	}
}

func TestLockfileUpdateSkillVersion(t *testing.T) {
	lf := NewLockfile()

	slug := "test-skill"
	lf.AddSkill(slug, "Test", "1.0.0", "sha256:abc", []string{"latest"})

	// Update version
	newVersion := "1.1.0"
	newHash := "sha256:def"
	newTags := []string{"latest", "updated"}

	lf.UpdateSkillVersion(slug, newVersion, newHash, newTags)

	version, ok := lf.GetSkillVersion(slug)
	if !ok {
		t.Fatal("expected skill to exist")
	}

	if version != newVersion {
		t.Errorf("expected version %s, got %s", newVersion, version)
	}

	hash, ok := lf.GetSkillHash(slug)
	if !ok {
		t.Fatal("expected hash to exist")
	}

	if hash != newHash {
		t.Errorf("expected hash %s, got %s", newHash, hash)
	}
}

func TestLockfileSkillCount(t *testing.T) {
	lf := NewLockfile()

	if lf.SkillCount() != 0 {
		t.Errorf("expected 0 skills, got %d", lf.SkillCount())
	}

	lf.AddSkill("skill1", "Skill 1", "1.0.0", "sha256:1", []string{})
	lf.AddSkill("skill2", "Skill 2", "1.0.0", "sha256:2", []string{})

	if lf.SkillCount() != 2 {
		t.Errorf("expected 2 skills, got %d", lf.SkillCount())
	}
}

func TestLockfileSaveLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "clawhub-lockfile-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and save lockfile
	lf := NewLockfile()
	lf.AddSkill("test-skill", "Test Skill", "1.0.0", "sha256:abc123", []string{"latest"})

	if err := lf.Save(tmpDir); err != nil {
		t.Fatalf("failed to save lockfile: %v", err)
	}

	// Load lockfile
	loaded, err := LoadLockfile(tmpDir)
	if err != nil {
		t.Fatalf("failed to load lockfile: %v", err)
	}

	if loaded.Version != lf.Version {
		t.Errorf("expected version %s, got %s", lf.Version, loaded.Version)
	}

	if loaded.SkillCount() != lf.SkillCount() {
		t.Errorf("expected %d skills, got %d", lf.SkillCount(), loaded.SkillCount())
	}

	skill, ok := loaded.GetSkill("test-skill")
	if !ok {
		t.Fatal("expected skill to exist")
	}

	if skill.Name != "Test Skill" {
		t.Errorf("expected name 'Test Skill', got %s", skill.Name)
	}
}

func TestLockfileInstalledAt(t *testing.T) {
	lf := NewLockfile()

	slug := "test-skill"
	before := time.Now()

	lf.AddSkill(slug, "Test", "1.0.0", "sha256:abc", []string{})

	after := time.Now()

	skill, ok := lf.GetSkill(slug)
	if !ok {
		t.Fatal("expected skill to exist")
	}

	if skill.InstalledAt.Before(before) || skill.InstalledAt.After(after) {
		t.Errorf("InstalledAt time %v is outside expected range [%v, %v]",
			skill.InstalledAt, before, after)
	}
}

func TestLoadLockfileNew(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "clawhub-lockfile-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Load non-existent lockfile
	lf, err := LoadLockfile(tmpDir)
	if err != nil {
		t.Fatalf("failed to load non-existent lockfile: %v", err)
	}

	if lf.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", lf.Version)
	}

	if lf.SkillCount() != 0 {
		t.Errorf("expected 0 skills, got %d", lf.SkillCount())
	}
}

func TestLockfileListSkills(t *testing.T) {
	lf := NewLockfile()

	lf.AddSkill("skill1", "Skill 1", "1.0.0", "sha256:1", []string{})
	lf.AddSkill("skill2", "Skill 2", "1.0.0", "sha256:2", []string{})

	skills := lf.ListSkills()

	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}

	if _, ok := skills["skill1"]; !ok {
		t.Error("expected skill1 to exist")
	}

	if _, ok := skills["skill2"]; !ok {
		t.Error("expected skill2 to exist")
	}
}
