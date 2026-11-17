package projects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchUsesConfiguredRootAndSkipsSensitiveFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "vendor"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("The billing service uses SQLite for local memory."), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("billing_password=not-for-the-model"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "vendor", "dependency.go"), []byte("billing dependency code"), 0600))

	configPath := filepath.Join(t.TempDir(), "projects.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"sample-project":`+quoteJSON(root)+`}`), 0600))
	catalog, err := Load(configPath)
	require.NoError(t, err)
	require.Equal(t, []string{"sample-project"}, catalog.Names())

	result, err := catalog.Search("SAMPLE-PROJECT", "How is billing memory stored?", 5)
	require.NoError(t, err)
	require.Contains(t, result, "README.md")
	require.Contains(t, result, "SQLite")
	require.NotContains(t, result, "not-for-the-model")
	require.NotContains(t, result, "dependency.go")
}

func TestLoadRejectsInvalidRepositoryRoot(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "projects.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"bad project":"/not/a/repository"}`), 0600))
	_, err := Load(configPath)
	require.ErrorContains(t, err, "invalid project name")
}

func quoteJSON(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
