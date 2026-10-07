package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// bundleID is the CFBundleIdentifier of the rolle app.
const bundleID = "com.getrolle.app"

// checkStaged makes sure that the staged update is the right kind of file
// for goos before it replaces the installed app. On macOS it must be a
// rolle .app bundle of the given version. On Windows it must be a PE
// executable, on Linux an ELF executable.
func checkStaged(goos, staged, version string) error {
	switch goos {
	case "darwin":
		if filepath.Ext(staged) != ".app" {
			return fmt.Errorf("update: staged %s is not an app bundle", filepath.Base(staged))
		}
		keys, err := plistStrings(filepath.Join(staged, "Contents", "Info.plist"))
		if err != nil {
			return fmt.Errorf("update: staged bundle: %w", err)
		}
		if keys["CFBundleIdentifier"] != bundleID {
			return fmt.Errorf("update: staged bundle is %q, not %s", keys["CFBundleIdentifier"], bundleID)
		}
		if got := keys["CFBundleShortVersionString"]; got != version {
			return fmt.Errorf("update: staged bundle is version %q, the release is %s", got, version)
		}
		return nil
	case "windows":
		return checkMagic(staged, []byte("MZ"), "a Windows executable")
	case "linux":
		return checkMagic(staged, []byte("\x7fELF"), "a Linux executable")
	}
	return nil
}

// checkMagic makes sure that path is a regular file that starts with magic.
func checkMagic(path string, magic []byte, kind string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return fmt.Errorf("update: staged %s is not a file", filepath.Base(path))
	}
	head := make([]byte, len(magic))
	if _, err := io.ReadFull(f, head); err != nil || !bytes.Equal(head, magic) {
		return fmt.Errorf("update: staged %s is not %s", filepath.Base(path), kind)
	}
	return nil
}

// plistStrings reads the string values of the top-level dict in an XML
// property list. It ignores nested dicts and values of other types.
func plistStrings(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	out := map[string]string{}
	depth, key := 0, ""
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse Info.plist: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "dict" {
				depth++
				key = ""
				continue
			}
			if depth != 1 {
				continue
			}
			if t.Name.Local != "key" && t.Name.Local != "string" {
				// A value of another type. Skip it and its children.
				key = ""
				if err := dec.Skip(); err != nil {
					return nil, fmt.Errorf("parse Info.plist: %w", err)
				}
				continue
			}
			var text string
			if err := dec.DecodeElement(&text, &t); err != nil {
				return nil, fmt.Errorf("parse Info.plist: %w", err)
			}
			if t.Name.Local == "key" {
				key = text
			} else if key != "" {
				out[key] = strings.TrimSpace(text)
				key = ""
			}
		case xml.EndElement:
			if t.Name.Local == "dict" {
				depth--
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no top-level strings in Info.plist")
	}
	return out, nil
}
