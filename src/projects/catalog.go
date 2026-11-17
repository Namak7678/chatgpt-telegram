package projects

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Catalog struct {
	roots map[string]string
}

type match struct {
	path  string
	score int
	lines []string
}

var excludedDirs = map[string]bool{
	".git": true, ".refact": true, ".second-brain": true, ".buddy": true, "node_modules": true,
	"vendor": true, "build": true, "dist": true, "target": true, "out": true,
	"credentials": true, "secrets": true, "private": true,
}

var textExtensions = map[string]bool{
	".c": true, ".cc": true, ".cpp": true, ".cs": true, ".css": true, ".go": true,
	".h": true, ".html": true, ".java": true, ".js": true, ".jsx": true, ".json": true,
	".md": true, ".py": true, ".rb": true, ".rs": true, ".sh": true, ".sql": true,
	".toml": true, ".ts": true, ".tsx": true, ".txt": true, ".xml": true,
	".yaml": true, ".yml": true,
}

var excludedNameParts = []string{
	".env", "projects.json", "secret", "credential", "password", "token", "private_key",
	"private-key", "id_rsa", "id_ed25519",
}

var excludedExtensions = map[string]bool{
	".crt": true, ".key": true, ".pem": true, ".p12": true, ".pfx": true,
	".sqlite": true, ".db": true,
}

var stopWords = map[string]bool{
	"about": true, "after": true, "also": true, "and": true, "are": true,
	"can": true, "for": true, "from": true, "how": true, "into": true,
	"its": true, "not": true, "our": true, "that": true, "the": true,
	"their": true, "this": true, "was": true, "what": true, "when": true,
	"where": true, "which": true, "who": true, "with": true, "you": true,
	"your": true,
}

func Load(path string) (*Catalog, error) {
	catalog := &Catalog{roots: make(map[string]string)}
	if path == "" {
		return catalog, nil
	}

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open projects file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(io.LimitReader(file, 64*1024))
	var configured map[string]string
	if err := decoder.Decode(&configured); err != nil && err != io.EOF {
		return nil, fmt.Errorf("decode projects file: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("projects file contains trailing data")
	}

	for slug, root := range configured {
		if !validSlug(slug) {
			return nil, fmt.Errorf("invalid project name %q; use letters, numbers, underscores, or hyphens", slug)
		}
		if _, exists := catalog.CanonicalName(slug); exists {
			return nil, fmt.Errorf("project names must be unique regardless of letter case: %q", slug)
		}
		if strings.TrimSpace(root) == "" {
			return nil, fmt.Errorf("project %q has an empty repository path", slug)
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve path for project %q: %w", slug, err)
		}
		info, err := os.Stat(absoluteRoot)
		if err != nil {
			return nil, fmt.Errorf("open repository path for project %q: %w", slug, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("repository path for project %q is not a directory", slug)
		}
		catalog.roots[slug] = absoluteRoot
	}
	return catalog, nil
}

func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.roots))
	for slug := range c.roots {
		names = append(names, slug)
	}
	sort.Strings(names)
	return names
}

func (c *Catalog) Has(slug string) bool {
	_, ok := c.CanonicalName(slug)
	return ok
}

func (c *Catalog) CanonicalName(slug string) (string, bool) {
	if _, ok := c.roots[slug]; ok {
		return slug, true
	}
	for name := range c.roots {
		if strings.EqualFold(name, slug) {
			return name, true
		}
	}
	return "", false
}

func (c *Catalog) Search(slug, query string, maxFiles int) (string, error) {
	root, ok := c.root(slug)
	if !ok {
		return "", fmt.Errorf("unknown project %q", slug)
	}
	terms := queryTerms(query)
	if len(terms) == 0 {
		return "", nil
	}

	var matches []match
	visited := 0
	var bytesRead int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && excludedDirs[strings.ToLower(entry.Name())] {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !isSearchable(entry.Name()) {
			return nil
		}
		visited++
		if visited > 5000 {
			return errFileLimitReached
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 1024*1024 || bytesRead+info.Size() > 16*1024*1024 {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		bytesRead += int64(len(content))
		if strings.ContainsRune(string(content), '\x00') {
			return nil
		}
		fileMatch := scoreFile(root, path, string(content), terms)
		if fileMatch.score > 0 {
			matches = append(matches, fileMatch)
		}
		return nil
	})
	if err != nil && !errors.Is(err, errFileLimitReached) {
		return "", fmt.Errorf("search project %q: %w", slug, err)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].path < matches[j].path
		}
		return matches[i].score > matches[j].score
	})
	if maxFiles <= 0 {
		maxFiles = 5
	}
	if len(matches) > maxFiles {
		matches = matches[:maxFiles]
	}

	var context strings.Builder
	for _, file := range matches {
		fmt.Fprintf(&context, "File: %s\n", file.path)
		for _, line := range file.lines {
			fmt.Fprintf(&context, "  %s\n", line)
		}
	}
	return strings.TrimSpace(context.String()), nil
}

var errFileLimitReached = errors.New("project search file limit reached")

func (c *Catalog) root(slug string) (string, bool) {
	name, ok := c.CanonicalName(slug)
	if !ok {
		return "", false
	}
	return c.roots[name], true
}

func validSlug(slug string) bool {
	if slug == "" {
		return false
	}
	for _, r := range slug {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func isSearchable(name string) bool {
	lowerName := strings.ToLower(name)
	for _, part := range excludedNameParts {
		if strings.Contains(lowerName, part) {
			return false
		}
	}
	extension := strings.ToLower(filepath.Ext(name))
	return textExtensions[extension] && !excludedExtensions[extension]
}

func queryTerms(query string) []string {
	seen := make(map[string]bool)
	var terms []string
	for _, word := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(word)) < 2 || stopWords[word] || seen[word] {
			continue
		}
		seen[word] = true
		terms = append(terms, word)
	}
	return terms
}

func scoreFile(root, path, content string, terms []string) match {
	file := match{}
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		relativePath = filepath.Base(path)
	}
	file.path = filepath.ToSlash(relativePath)
	for lineNumber, line := range strings.Split(content, "\n") {
		lowerLine := strings.ToLower(line)
		for _, term := range terms {
			file.score += strings.Count(lowerLine, term)
		}
		matched := false
		for _, term := range terms {
			if strings.Contains(lowerLine, term) {
				matched = true
				break
			}
		}
		if matched && len(file.lines) < 3 {
			if len(line) > 400 {
				line = line[:400]
			}
			file.lines = append(file.lines, fmt.Sprintf("%d: %s", lineNumber+1, strings.TrimSpace(line)))
		}
	}
	return file
}
