package downloads

import (
	"context"
	"testing"
)

type completionNotifier struct {
	completed []DownloadCompletedEvent
}

func (n *completionNotifier) NotifyDownloadComplete(clientID, downloadID, title, category, contentPath, savePath string) {
	n.completed = append(n.completed, DownloadCompletedEvent{
		ClientID: clientID, DownloadID: downloadID, Title: title,
		Category: category, ContentPath: contentPath, SavePath: savePath,
	})
}

func (*completionNotifier) NotifyDownloadProgress(string, string, float64, int64, int64, float64, string, string, string) {
}

func TestMonitorRedeliversCompletionToOrchestratorDespiteHistory(t *testing.T) {
	bus := &eventbusStub{}
	monitor := newTestMonitor(t, bus)
	monitor.historyStore = openHistoryTestStore(t)
	notifier := &completionNotifier{}
	monitor.orchNotifier = notifier
	ctx := context.Background()
	ev := &DownloadCompletedEvent{ClientID: "client", DownloadID: "download"}
	if err := monitor.historyStore.RecordCompletion(ctx, ev); err != nil {
		t.Fatal(err)
	}
	items := []Item{{
		ClientID: "client", ID: "download", Status: StatusItemSeeding,
		Title: "Movie", ContentPath: "/downloads/Movie",
	}}
	monitor.emitCompletions(ctx, items)
	monitor.emitCompletions(ctx, items)
	if len(notifier.completed) != 2 || notifier.completed[0].ContentPath != "/downloads/Movie" {
		t.Fatalf("completion history suppressed recoverable workflow delivery: %+v", notifier.completed)
	}
	if len(bus.events) != 0 {
		t.Fatalf("download events must still be deduplicated: %d", len(bus.events))
	}
}
