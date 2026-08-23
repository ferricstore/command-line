package architecture

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestHTTPAndTLSDependenciesStayBehindFerricBoundary(t *testing.T) {
	t.Parallel()

	internalRoot := internalDirectory(t)
	forbidden := map[string]struct{}{
		"crypto/tls":  {},
		"crypto/x509": {},
		"net/http":    {},
	}
	err := filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == filepath.Join(internalRoot, "ferric") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if _, blocked := forbidden[name]; blocked {
				t.Errorf("%s imports %s; HTTP/TLS construction belongs in internal/ferric", path, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTPPasswordBoundaryDoesNotAddTenantMetadataOrCustomHeaders(t *testing.T) {
	t.Parallel()

	path := filepath.Join(internalDirectory(t), "ferric", "password.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ToLower(string(contents))
	if strings.Contains(source, "tenant") {
		t.Fatal("HTTP password boundary contains tenant-specific behavior")
	}
	if strings.Contains(source, "withhttpheaders") {
		t.Fatal("HTTP password boundary adds custom headers")
	}
}

func TestCIMatrixIncludesExactMinimumGoVersion(t *testing.T) {
	t.Parallel()

	repositoryRoot := filepath.Dir(internalDirectory(t))
	goMod, err := os.ReadFile(filepath.Join(repositoryRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	minimumVersion := ""
	for _, line := range strings.Split(string(goMod), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			minimumVersion = fields[1]
			break
		}
	}
	if minimumVersion == "" {
		t.Fatal("go.mod does not declare a minimum Go version")
	}

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workflow), "- "+minimumVersion) {
		t.Fatalf("CI matrix must include exact go.mod minimum Go version %s", minimumVersion)
	}
}

func internalDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}
