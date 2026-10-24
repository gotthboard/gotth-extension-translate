package translate

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSourceHasNoRemoteEgressMechanism(t *testing.T) {
	root := filepath.Join("..", "..")
	forbiddenImports := map[string]bool{"net/http": true, "net/url": true, "crypto/tls": true, "os/exec": true, "plugin": true}
	forbiddenLiterals := []string{"http://", "https://", "DialTCP", "DialUDP"}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		if entry.IsDir() {
			if relative == ".git" || strings.HasPrefix(relative, ".artifacts") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, ".pb.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, item := range file.Imports {
			value := strings.Trim(item.Path.Value, string(rune(34)))
			if forbiddenImports[value] {
				t.Errorf("remote egress import %s in %s", value, relative)
			}
		}
		source := string(data)
		for _, item := range forbiddenLiterals {
			if strings.Contains(source, item) {
				t.Errorf("remote egress literal %s in %s", item, relative)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("backend.go")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "backend.go", data, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = parsed
	source := string(data)
	if !strings.Contains(source, "SO_PEERCRED") || !strings.Contains(source, "net.FileConn") || !strings.Contains(source, "backend reconnect forbidden") || !strings.Contains(source, "grpc.WithDisableRetry()") || !strings.Contains(source, "grpc.WithIdleTimeout(0)") {
		t.Fatal("backend adapter is not pinned to one authenticated connected Unix peer")
	}
	if strings.Contains(source, "net.Dial") || strings.Contains(source, "DialUnix") {
		t.Fatal("backend adapter may not dial a mutable pathname")
	}
}
