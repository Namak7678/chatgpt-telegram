package memory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorePersistsHistoryAndScopesNotes(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.SaveMessage(42, "user", "first"))
	require.NoError(t, store.SaveMessage(42, "assistant", "second"))
	require.NoError(t, store.SaveMessage(7, "user", "another chat"))
	history, err := store.History(42, 10)
	require.NoError(t, err)
	require.Equal(t, []Message{{Role: "user", Content: "first"}, {Role: "assistant", Content: "second"}}, history)

	globalID, err := store.AddNote(42, "", "global note")
	require.NoError(t, err)
	_, err = store.AddNote(42, "project-a", "project note")
	require.NoError(t, err)
	_, err = store.AddNote(7, "project-a", "private to another chat")
	require.NoError(t, err)
	require.NoError(t, store.Close())
	store, err = Open(dataDir)
	require.NoError(t, err)

	notes, err := store.Notes(42, "project-a", 10)
	require.NoError(t, err)
	require.Len(t, notes, 2)
	notes, err = store.Notes(42, "", 10)
	require.NoError(t, err)
	require.Len(t, notes, 1)

	forgotten, err := store.ForgetNote(7, globalID)
	require.NoError(t, err)
	require.False(t, forgotten)
	forgotten, err = store.ForgetNote(42, globalID)
	require.NoError(t, err)
	require.True(t, forgotten)

	require.NoError(t, store.ResetConversation(42))
	history, err = store.History(42, 10)
	require.NoError(t, err)
	require.Empty(t, history)
}
