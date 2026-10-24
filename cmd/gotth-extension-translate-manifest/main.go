package main

import (
	"fmt"
	"os"

	"github.com/gotthboard/gotth-extension-translate/internal/translate"
	extensioncore "github.com/gotthboard/gotth-extensions/pkg/extensions"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gotth-extension-translate-manifest <version>")
		os.Exit(2)
	}
	manifest, err := translate.Manifest(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid version")
		os.Exit(2)
	}
	encoded, err := extensioncore.CanonicalManifest(manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, "manifest unavailable")
		os.Exit(1)
	}
	_, _ = os.Stdout.Write(encoded)
}
