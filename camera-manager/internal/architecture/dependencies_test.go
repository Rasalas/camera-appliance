package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Business modules must stay usable without the application or HTTP adapters.
// Pure layout/routing code may return decisions, but must not perform I/O itself.
func TestModuleDependencyDirection(t *testing.T) {
	for _, module := range []string{"display", "streamrouting", "relay", "cameraaccess", "snapshotupload", "releasearchive", "update"} {
		t.Run(module, func(t *testing.T) {
			files, err := os.ReadDir(filepath.Join("..", module))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range files {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", module, entry.Name()), nil, parser.ImportsOnly)
				if err != nil {
					t.Fatal(err)
				}
				for _, spec := range file.Imports {
					imported, _ := strconv.Unquote(spec.Path.Value)
					for _, upstream := range []string{"app", "web", "cli"} {
						prefix := "camera-appliance/camera-manager/internal/" + upstream
						if imported == prefix || strings.HasPrefix(imported, prefix+"/") {
							t.Errorf("%s/%s imports upstream %s", module, entry.Name(), imported)
						}
					}
					if module == "display" || module == "streamrouting" {
						for _, ioPackage := range []string{"os", "os/exec", "net", "net/http", "database/sql"} {
							if imported == ioPackage {
								t.Errorf("pure module %s imports %s", module, imported)
							}
						}
					}
				}
			}
		})
	}
}
