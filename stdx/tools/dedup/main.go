package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileInfo struct {
	Path string
	Hash [32]byte
	Size int64
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: dedup <folder1> [<folder2> ...]")
		os.Exit(1)
	}

	if err := findDuplicates(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func findDuplicates(folders []string) (err error) {
	fileMap := make(map[[32]byte][]FileInfo)

	for _, folder := range folders {
		err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			hash, err := hashFile(path)
			if err != nil {
				return fmt.Errorf("Error hashing file %s: %w\n", path, err)
			}

			fileSize := info.Size()
			fileInfo := FileInfo{
				Path: path,
				Hash: hash,
				Size: fileSize,
			}

			if existingFiles, ok := fileMap[hash]; ok && len(existingFiles) > 0 {
				if existingFiles[0].Size != fileSize {
					fmt.Printf("Found hash collision: %s", hex.EncodeToString(hash[:]))
					fmt.Printf("  %s (size: %d bytes)", path, fileSize)
					for _, existingFile := range existingFiles {
						fmt.Printf("  %s (size: %d bytes)", existingFile.Path, existingFile.Size)
					}
				}
			}

			fileMap[hash] = append(fileMap[hash], fileInfo)
			return nil
		})

		if err != nil {
			return fmt.Errorf("error walking folder %s: %w", folder, err)
		}
	}

	// Find and print duplicates
	foundDuplicates := false

	// Collect all duplicate groups
	duplicateGroups := make([][]FileInfo, 0, min(1, len(fileMap)/10))
	for _, files := range fileMap {
		if len(files) > 1 {
			foundDuplicates = true
			duplicateGroups = append(duplicateGroups, files)
		}
	}

	// Sort duplicate groups by size in descending order (larger files first)
	sort.Slice(duplicateGroups, func(i, j int) bool {
		return duplicateGroups[i][0].Size > duplicateGroups[j][0].Size
	})

	// Print duplicates in reverse size order
	for _, files := range duplicateGroups {
		fmt.Printf("\nhash: %s, size: %d bytes:\n", hex.EncodeToString(files[0].Hash[:]), files[0].Size)
		// Sort by path for consistent output within each group
		sort.Slice(files, func(i, j int) bool {
			return strings.Compare(files[i].Path, files[j].Path) < 0
		})
		for _, file := range files {
			fmt.Println(" ", file.Path)
		}
	}

	if !foundDuplicates {
		fmt.Println("No duplicate files found.")
	}

	return nil
}

func hashFile(filePath string) ([32]byte, error) {
	var hash [32]byte

	file, err := os.Open(filePath)
	if err != nil {
		return hash, err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return hash, err
	}

	hasher.Sum(hash[:0])

	return hash, nil
}
