package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/ollama/ollama/types/model"
)

type Manifest struct {
	SchemaVersion int     `json:"schemaVersion"`
	MediaType     string  `json:"mediaType"`
	Config        Layer   `json:"config"`
	Layers        []Layer `json:"layers"`

	filepath string
	fi       os.FileInfo
	digest   string
}

func (m *Manifest) Size() (size int64) {
	fmt.Printf("Calculating manifest size for %s\n", m.filepath)
	for _, layer := range append(m.Layers, m.Config) {
		size += layer.Size
	}
	fmt.Printf("Total manifest size: %d bytes\n", size)
	return
}

func (m *Manifest) Remove() error {
	fmt.Printf("Removing manifest file: %s\n", m.filepath)
	if err := os.Remove(m.filepath); err != nil {
		fmt.Printf("Error removing manifest file: %v\n", err)
		return err
	}

	manifests, err := GetManifestPath()
	if err != nil {
		fmt.Printf("Error getting manifest path: %v\n", err)
		return err
	}

	fmt.Println("Pruning empty directories in manifest path")
	return PruneDirectory(manifests)
}

func (m *Manifest) RemoveLayers() error {
	fmt.Printf("Removing layers for manifest %s\n", m.filepath)
	for _, layer := range append(m.Layers, m.Config) {
		if layer.Digest != "" {
			fmt.Printf("Removing layer with digest: %s\n", layer.Digest)
			if err := layer.Remove(); errors.Is(err, os.ErrNotExist) {
				slog.Debug("layer does not exist", "digest", layer.Digest)
			} else if err != nil {
				fmt.Printf("Error removing layer %s: %v\n", layer.Digest, err)
				return err
			}
		}
	}
	fmt.Println("All layers removed successfully")
	return nil
}

func ParseNamedManifest(n model.Name) (*Manifest, error) {
	fmt.Printf("Parsing named manifest: %s\n", n)
	if !n.IsFullyQualified() {
		fmt.Printf("Name is not fully qualified: %s\n", n)
		return nil, model.Unqualified(n)
	}

	manifests, err := GetManifestPath()
	if err != nil {
		fmt.Printf("Error getting manifest path: %v\n", err)
		return nil, err
	}

	p := filepath.Join(manifests, n.Filepath())
	fmt.Printf("Manifest file path: %s\n", p)

	var m Manifest
	f, err := os.Open(p)
	if err != nil {
		fmt.Printf("Error opening manifest file: %v\n", err)
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		fmt.Printf("Error getting file info: %v\n", err)
		return nil, err
	}

	sha256sum := sha256.New()
	if err := json.NewDecoder(io.TeeReader(f, sha256sum)).Decode(&m); err != nil {
		fmt.Printf("Error decoding manifest JSON: %v\n", err)
		return nil, err
	}

	m.filepath = p
	m.fi = fi
	m.digest = hex.EncodeToString(sha256sum.Sum(nil))
	fmt.Printf("Manifest parsed successfully, digest: %s\n", m.digest)

	return &m, nil
}

func WriteManifest(name model.Name, config Layer, layers []Layer) error {
	fmt.Printf("Writing manifest for: %s\n", name)
	manifests, err := GetManifestPath()
	if err != nil {
		fmt.Printf("Error getting manifest path: %v\n", err)
		return err
	}

	p := filepath.Join(manifests, name.Filepath())
	fmt.Printf("Writing manifest to: %s\n", p)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		fmt.Printf("Error creating directories: %v\n", err)
		return err
	}

	f, err := os.Create(p)
	if err != nil {
		fmt.Printf("Error creating manifest file: %v\n", err)
		return err
	}
	defer f.Close()

	m := Manifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.docker.distribution.manifest.v2+json",
		Config:        config,
		Layers:        layers,
	}

	fmt.Println("Encoding manifest to JSON")
	if err := json.NewEncoder(f).Encode(m); err != nil {
		fmt.Printf("Error encoding manifest: %v\n", err)
		return err
	}

	fmt.Println("Manifest written successfully")
	return nil
}

func Manifests(continueOnError bool) (map[model.Name]*Manifest, error) {
	fmt.Println("Listing all manifests")
	manifests, err := GetManifestPath()
	if err != nil {
		fmt.Printf("Error getting manifest path: %v\n", err)
		return nil, err
	}

	fmt.Printf("Searching for manifests in: %s\n", manifests)
	matches, err := filepath.Glob(filepath.Join(manifests, "*", "*", "*", "*"))
	if err != nil {
		fmt.Printf("Error globbing manifest files: %v\n", err)
		return nil, err
	}

	ms := make(map[model.Name]*Manifest)
	fmt.Printf("Found %d potential manifest files\n", len(matches))

	for _, match := range matches {
		fmt.Printf("Processing manifest candidate: %s\n", match)
		fi, err := os.Stat(match)
		if err != nil {
			fmt.Printf("Error stating file: %v\n", err)
			return nil, err
		}

		if !fi.IsDir() {
			rel, err := filepath.Rel(manifests, match)
			if err != nil {
				if !continueOnError {
					fmt.Printf("Error getting relative path: %v\n", err)
					return nil, fmt.Errorf("%s %w", match, err)
				}
				slog.Warn("bad filepath", "path", match, "error", err)
				fmt.Printf("Skipping bad filepath: %s (error: %v)\n", match, err)
				continue
			}

			n := model.ParseNameFromFilepath(rel)
			if !n.IsValid() {
				if !continueOnError {
					fmt.Printf("Invalid manifest name: %s\n", rel)
					return nil, fmt.Errorf("%s %w", rel, err)
				}
				slog.Warn("bad manifest name", "path", rel)
				fmt.Printf("Skipping invalid manifest name: %s\n", rel)
				continue
			}

			m, err := ParseNamedManifest(n)
			if err != nil {
				if !continueOnError {
					fmt.Printf("Error parsing manifest: %v\n", err)
					return nil, fmt.Errorf("%s %w", n, err)
				}
				slog.Warn("bad manifest", "name", n, "error", err)
				fmt.Printf("Skipping bad manifest %s (error: %v)\n", n, err)
				continue
			}

			fmt.Printf("Adding manifest to results: %s\n", n)
			ms[n] = m
		}
	}

	fmt.Printf("Found %d valid manifests\n", len(ms))
	return ms, nil
}
