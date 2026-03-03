package util

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/rs/zerolog/log"
)

// FindFilesWithPatterns finds files matching the regex pattern in the specified directory
// directory: path to search
// pattern: regex pattern
// recursive: whether to search subdirectories recursively
// Returns matched file paths and any error
func FindFilesWithPatterns(directory string, pattern string, recursive bool) ([]string, error) {
	// Compile regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex '%s': %v", pattern, err)
	}

	// Check if directory exists
	dirInfo, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("cannot access directory '%s': %v", directory, err)
	}
	if !dirInfo.IsDir() {
		return nil, fmt.Errorf("'%s' is not a directory", directory)
	}

	// Store matched file paths
	var matchedFiles []string

	// Create file system
	fsys := os.DirFS(directory)

	// Walk file system
	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// If directory and not recursive, skip subdirectories
		if d.IsDir() {
			if !recursive && path != "." {
				return fs.SkipDir
			}
			return nil
		}

		// Check if filename matches the regex
		if re.MatchString(d.Name()) {
			// Add full path to result list
			fullPath := filepath.Join(directory, path)
			matchedFiles = append(matchedFiles, fullPath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory: %v", err)
	}

	return matchedFiles, nil
}

func DefaultWorkDir(account string) string {
	if len(account) == 0 {
		switch runtime.GOOS {
		case "windows":
			return filepath.Join(os.ExpandEnv("${USERPROFILE}"), "Documents", "chatlog")
		case "darwin":
			return filepath.Join(os.ExpandEnv("${HOME}"), "Documents", "chatlog")
		default:
			return filepath.Join(os.ExpandEnv("${HOME}"), "chatlog")
		}
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.ExpandEnv("${USERPROFILE}"), "Documents", "chatlog", account)
	case "darwin":
		return filepath.Join(os.ExpandEnv("${HOME}"), "Documents", "chatlog", account)
	default:
		return filepath.Join(os.ExpandEnv("${HOME}"), "chatlog", account)
	}
}

func GetDirSize(dir string) string {
	var size int64
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			size += info.Size()
		}
		return nil
	})
	return ByteCountSI(size)
}

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

// PrepareDir ensures that the specified directory path exists.
// If the directory does not exist, it attempts to create it.
func PrepareDir(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		} else {
			return err
		}
	} else if !stat.IsDir() {
		log.Debug().Msgf("%s is not a directory", path)
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
