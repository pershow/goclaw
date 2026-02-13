package skills

// MaxSkillNameLength is the maximum allowed length for skill names.
const MaxSkillNameLength = 64

// MaxSkillDescriptionLength is the maximum allowed length for skill descriptions.
const MaxSkillDescriptionLength = 1024

// DefaultSkillStartDelimiter is the delimiter that starts the YAML frontmatter block.
const DefaultSkillStartDelimiter = "---"

// DefaultSkillEndDelimiter is the delimiter that ends the YAML frontmatter block.
const DefaultSkillEndDelimiter = "---"

// DefaultInstallTimeout is the default timeout for skill dependency installations.
const DefaultInstallTimeout = 5 * 60 // 5 minutes in seconds

// Supported skill frontmatter fields
const (
	// Core fields required by Agent Skills spec
	FieldName        = "name"
	FieldDescription = "description"

	// Optional core fields
	FieldVersion       = "version"
	FieldAuthor        = "author"
	FieldHomepage      = "homepage"
	FieldAlways        = "always"
	FieldLicense       = "license"
	FieldCompatibility = "compatibility"
	FieldMetadata      = "metadata"
	FieldAllowedTools  = "allowed-tools"

	// Skill invocation control fields
	FieldDisableModelInvocation = "disable-model-invocation"
	FieldUserInvocable          = "user-invocable"

	// Command dispatch fields for slash commands
	FieldCommandDispatch = "command-dispatch"
	FieldCommandTool     = "command-tool"
	FieldCommandArgMode  = "command-arg-mode"
)

// Valid skill name characters (lowercase letters, digits, hyphens only).
var ValidSkillNameChars = "abcdefghijklmnopqrstuvwxyz0123456789-"

// Valid skill install kinds.
var ValidInstallKinds = []string{
	"brew",     // Homebrew formula installation
	"node",     // Node.js package installation (npm/pnpm/yarn/bun)
	"go",       // Go module installation
	"uv",       // Python package installation via uv
	"download", // Direct URL download with optional extraction
	"apt",      // APT package installation (legacy)
	"pip",      // Python pip installation (legacy)
	"npm",      // NPM installation (legacy)
	"pnpm",     // PNPM installation (legacy)
	"yarn",     // Yarn installation (legacy)
	"bun",      // Bun installation (legacy)
	"command",  // Custom command execution
}

// Valid Node.js package managers for installation.
var ValidNodeManagers = []string{
	"npm",
	"pnpm",
	"yarn",
	"bun",
}

// Valid command dispatch modes.
var ValidArgModes = []string{
	"raw", // Forward raw argument string without parsing
}

// Valid operating systems for skill dependencies.
var ValidOperatingSystems = []string{
	"darwin",  // macOS
	"linux",   // Linux
	"windows", // Windows
	"freebsd", // FreeBSD
}

// Diagnostic types for skill validation.
const (
	DiagnosticTypeWarning   = "warning"
	DiagnosticTypeError     = "error"
	DiagnosticTypeCollision = "collision"
)

// Default skill file names (in priority order).
var DefaultSkillFileNames = []string{
	"SKILL.md", // Primary format (per Agent Skills standard)
	"skill.md", // Alternative casing
}

// Ignored directory patterns when scanning for skills.
var IgnoredDirectoryPatterns = []string{
	"node_modules",  // Node.js dependencies
	".git",          // Git metadata
	".hg",           // Mercurial metadata
	".svn",          // Subversion metadata
	".DS_Store",     // macOS metadata
	"__pycache__",   // Python cache
	".pytest_cache", // Python test cache
}

// Ignored file patterns when scanning for skills.
var IgnoredFilePatterns = []string{
	".gitignore",
	".gitattributes",
	".gitmodules",
	".npmignore",
	".dockerignore",
	".prettierignore",
	".eslintignore",
}

// Ignore file names that affect directory scanning.
var IgnoreFileNames = []string{
	".gitignore",
	".ignore",
	".fdignore",
}

// Default skill source directories (in precedence order - highest first).
var DefaultSkillSourceDirs = []struct {
	Name     string
	Path     string
	Priority int
}{
	{"workspace", "skills", 300},            // Current project skills (highest priority)
	{"managed", "~/.goclaw/skills", 200},    // Global user skills
	{"bundled", "<executable>/skills", 100}, // Built-in skills from package
}

// Skill discovery rules constants.
const (
	// DiscoveryModeDirect indicates direct .md files in directory roots
	DiscoveryModeDirect = "direct"

	// DiscoveryModeRecursive indicates recursive SKILL.md files under subdirectories
	DiscoveryModeRecursive = "recursive"

	// DefaultDiscoveryMode is the default discovery strategy
	DefaultDiscoveryMode = DiscoveryModeRecursive
)

// Error messages for skill parsing and validation.
const (
	ErrMsgNameRequired        = "skill name is required"
	ErrMsgNameLength          = "skill name exceeds maximum length of %d characters"
	ErrMsgNameCharacters      = "skill name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)"
	ErrMsgNameHyphens         = "skill name must not start or end with a hyphen"
	ErrMsgNameConsecutive     = "skill name must not contain consecutive hyphens"
	ErrMsgNameMatchesDir      = "skill name must match parent directory name"
	ErrMsgDescriptionRequired = "skill description is required"
	ErrMsgDescriptionLength   = "skill description exceeds maximum length of %d characters"
	ErrMsgFrontmatterMissing  = "skill file must contain valid frontmatter"
	ErrMsgFrontmatterParse    = "failed to parse skill frontmatter"
	ErrMsgFileNotFound        = "skill file not found"
	ErrMsgDirectoryNotFound   = "skill directory not found"
	ErrMsgSkillDirectoryEmpty = "skill directory is empty"
)

// Installation error messages.
const (
	ErrMsgInstallUnsupported     = "unsupported install kind: %s"
	ErrMsgInstallBinaryMissing   = "required binary not found after installation: %s"
	ErrMsgInstallTimeout         = "installation timed out"
	ErrMsgInstallFailed          = "installation failed: %s"
	ErrMsgInstallOSNotSupported  = "installation not supported on %s"
	ErrMsgInstallCanceled        = "installation canceled by user"
	ErrMsgInstallSecurityWarning = "security warning during installation"
)

// Installation command templates by platform.
var InstallCommandTemplates = map[string]string{
	// Homebrew (macOS/Linux)
	"brew": "brew install %s",

	// Node.js package managers
	"npm":  "npm install -g %s",
	"pnpm": "pnpm add -g %s",
	"yarn": "yarn global add %s",
	"bun":  "bun add -g %s",

	// Python package managers
	"pip":  "pip install %s",
	"pip3": "pip3 install %s",
	"uv":   "uv pip install %s",

	// Go modules
	"go": "go install %s",

	// System package managers
	"apt":     "apt install -y %s",
	"apt-get": "apt-get install -y %s",
}

// Skill prompt template constants.
const (
	// Skill section header for system prompt
	SkillSectionHeader = "## Skills (mandatory)\n\n" +
		"Before replying: scan <available_skills> entries.\n" +
		"- If exactly one skill clearly applies: output a tool call `use_skill` with the skill name as parameter.\n" +
		"- If multiple could apply: choose the most specific one, then call `use_skill`.\n" +
		"- If none clearly apply: do not use any skill.\n" +
		"Constraints: only use one skill at a time; the skill content will be injected after selection.\n"

	// Skill block template for XML formatting
	SkillBlockTemplate = `<skill name="%s">\n` +
		`**Name:** %s\n` +
		`**Description:** %s\n` +
		`**Location:** %s\n` +
		`</skill>\n`

	// Selected skill section header
	SelectedSkillHeader = "## Selected Skills (active)\n\n"

	// Selected skill template with full content
	SelectedSkillTemplate = `<skill name="%s" location="%s">\n` +
		`References are relative to %s.\n\n` +
		`%s` +
		`</skill>\n`
)

// Skill command constants for slash command generation.
const (
	// Maximum command name length (for sanitization)
	MaxCommandNameLength = 32

	// Maximum command description length (for UI constraints)
	MaxCommandDescriptionLength = 100

	// Command name characters (simplified for command-line compatibility)
	ValidCommandNameChars = "abcdefghijklmnopqrstuvwxyz0123456789_"
)

// Environment configuration constants.
const (
	// Environment variables that control skill behavior
	EnvAutoInstall      = "GOCLAW_SKILL_AUTO_INSTALL"
	EnvSkillsDir        = "GOCLAW_SKILLS_DIR"
	EnvSkillsExtraDirs  = "GOCLAW_SKILLS_EXTRA_DIRS"
	EnvSkillsIgnoreDirs = "GOCLAW_SKILLS_IGNORE_DIRS"
)
