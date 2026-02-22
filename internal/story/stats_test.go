package story

import (
	"testing"

	"github.com/maxghenis/openmessage/internal/db"
)

func TestComputeStatsEmpty(t *testing.T) {
	stats := ComputeStats(nil)
	if stats.TotalMessages != 0 {
		t.Errorf("total = %d, want 0", stats.TotalMessages)
	}
}

func TestComputeStats(t *testing.T) {
	messages := []*db.Message{
		{MessageID: "1", SenderName: "Alice", Body: "Hello there!", TimestampMS: 1700000000000, IsFromMe: false},
		{MessageID: "2", SenderName: "", Body: "Hi Alice!", TimestampMS: 1700000060000, IsFromMe: true},
		{MessageID: "3", SenderName: "Alice", Body: "How are you doing today?", TimestampMS: 1700000120000, IsFromMe: false},
		{MessageID: "4", SenderName: "", Body: "Great, thanks!", TimestampMS: 1700000180000, IsFromMe: true},
		{MessageID: "5", SenderName: "Alice", Body: "Want to grab coffee?", TimestampMS: 1700000240000, IsFromMe: false},
	}

	stats := ComputeStats(messages)

	if stats.TotalMessages != 5 {
		t.Errorf("total = %d, want 5", stats.TotalMessages)
	}

	if stats.SenderSplit["Alice"] != 3 {
		t.Errorf("Alice count = %d, want 3", stats.SenderSplit["Alice"])
	}
	if stats.SenderSplit["me"] != 2 {
		t.Errorf("me count = %d, want 2", stats.SenderSplit["me"])
	}

	if len(stats.Yearly) != 1 {
		t.Fatalf("yearly count = %d, want 1", len(stats.Yearly))
	}
	if stats.Yearly[0].Year != "2023" {
		t.Errorf("year = %s, want 2023", stats.Yearly[0].Year)
	}

	// Hour heatmap should have 24*7 = 168 entries
	if len(stats.HourHeatmap) != 168 {
		t.Errorf("heatmap entries = %d, want 168", len(stats.HourHeatmap))
	}

	// Longest gap should be computed
	if stats.DateRange.Start == "" || stats.DateRange.End == "" {
		t.Error("date range should be set")
	}
}

func TestComputeStatsResponseTimes(t *testing.T) {
	// Alice sends, then "me" responds 5 minutes later
	messages := []*db.Message{
		{MessageID: "1", SenderName: "Alice", Body: "Hello", TimestampMS: 1700000000000, IsFromMe: false},
		{MessageID: "2", Body: "Hi", TimestampMS: 1700000300000, IsFromMe: true}, // 5 min later
		{MessageID: "3", SenderName: "Alice", Body: "What's up", TimestampMS: 1700000600000, IsFromMe: false}, // 5 min later
	}

	stats := ComputeStats(messages)

	// "me" responded to Alice in 5 minutes
	if rt, ok := stats.AvgResponseTimes["me"]; !ok || rt != 5 {
		t.Errorf("me avg response = %d, want 5", rt)
	}
	// Alice responded to me in 5 minutes
	if rt, ok := stats.AvgResponseTimes["Alice"]; !ok || rt != 5 {
		t.Errorf("Alice avg response = %d, want 5", rt)
	}
}

func TestComputeStatsLongestGap(t *testing.T) {
	messages := []*db.Message{
		{MessageID: "1", Body: "Hello", TimestampMS: 1700000000000, IsFromMe: true},
		// 10 day gap
		{MessageID: "2", Body: "Hi again", TimestampMS: 1700864000000, IsFromMe: true},
	}

	stats := ComputeStats(messages)

	if stats.LongestGap.Days != 10 {
		t.Errorf("longest gap = %d days, want 10", stats.LongestGap.Days)
	}
}

func TestGenerateLocalStory(t *testing.T) {
	messages := []*db.Message{
		{MessageID: "1", SenderName: "Alice", Body: "First message ever", TimestampMS: 1672531200000, IsFromMe: false},
		{MessageID: "2", Body: "Replying to you!", TimestampMS: 1672531260000, IsFromMe: true},
		{MessageID: "3", SenderName: "Alice", Body: "A year later message", TimestampMS: 1704067200000, IsFromMe: false},
	}

	story, err := Generate(messages, GenerateConfig{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if story.Title == "" {
		t.Error("title should not be empty")
	}
	if story.Stats == nil {
		t.Fatal("stats should not be nil")
	}
	if story.Stats.TotalMessages != 3 {
		t.Errorf("total = %d, want 3", story.Stats.TotalMessages)
	}
	if len(story.Chapters) == 0 {
		t.Error("should have at least one chapter")
	}
}
