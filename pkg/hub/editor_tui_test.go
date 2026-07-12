package hub

import (
	"errors"
	"testing"
)

// TestEditorInstallModel_StreamsLoadedError verifies that a stream load
// failure does not stop the flow: the model keeps an empty stream list and
// still proceeds to load releases (which skips the cache without streams).
func TestEditorInstallModel_StreamsLoadedError(t *testing.T) {
	m := initialEditorInstallModel(&Client{})

	updated, cmd := m.Update(streamsLoadedMsg{err: errors.New("network down")})

	model, ok := updated.(editorInstallModel)
	if !ok {
		t.Fatalf("Update returned %T, want editorInstallModel", updated)
	}
	if model.loadingStreams {
		t.Error("loadingStreams should be false after streamsLoadedMsg")
	}
	if len(model.streams) != 0 {
		t.Errorf("streams should be empty on error, got %d", len(model.streams))
	}
	if model.err != nil {
		t.Errorf("stream load failure should not become a fatal error, got %v", model.err)
	}
	if cmd == nil {
		t.Fatal("Update should return loadAllReleases cmd even when streams failed")
	}
}

// TestEditorInstallModel_StreamsLoadedSuccess verifies the sequential flow:
// loaded streams are stored and passed on to the release loading step.
func TestEditorInstallModel_StreamsLoadedSuccess(t *testing.T) {
	m := initialEditorInstallModel(&Client{})
	streams := []VersionStream{
		{MajorMinor: "6000.0", DisplayName: "Unity 6 (6000.0) LTS", TotalCount: 3, LTS: true, IsUnity6: true},
	}

	updated, cmd := m.Update(streamsLoadedMsg{streams: streams})

	model, ok := updated.(editorInstallModel)
	if !ok {
		t.Fatalf("Update returned %T, want editorInstallModel", updated)
	}
	if len(model.streams) != 1 || model.streams[0].MajorMinor != "6000.0" {
		t.Errorf("streams not stored, got %#v", model.streams)
	}
	if len(model.filteredStreams) != 1 {
		t.Errorf("filteredStreams not initialized, got %d", len(model.filteredStreams))
	}
	if cmd == nil {
		t.Fatal("Update should return loadAllReleases cmd after streams loaded")
	}
}
