package catalogsync

import (
	"context"
	"errors"
	"testing"

	"github.com/tycdn/vplayer/internal/store"
)

type classifyMem struct {
	rows  []store.UnclassifiedUnavailableEpisode
	marks []uint64
}

func (m *classifyMem) ListUnclassifiedUnavailable(afterID uint64, limit int) ([]store.UnclassifiedUnavailableEpisode, error) {
	var out []store.UnclassifiedUnavailableEpisode
	for _, row := range m.rows {
		if row.ID <= afterID {
			continue
		}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *classifyMem) MarkMembershipRestriction(id uint64) (bool, error) {
	m.marks = append(m.marks, id)
	return true, nil
}

func TestClassifyRestricted_marksOnlyExplicitMembership(t *testing.T) {
	restricted := `<html><body>系统提示 您没有权限访问此数据，请升级会员</body></html>`
	playable := `var player_aaaa={"url":"https://cdn.example/a.m3u8","encrypt":0}</script>`
	encrypted := `var player_aaaa={"url":"","encrypt":1}</script>`
	st := &classifyMem{rows: []store.UnclassifiedUnavailableEpisode{
		{ID: 10, VideoID: 10056, SourceVideoID: "32222", PlayPageURL: "https://src/vip"},
		{ID: 11, VideoID: 2, PlayPageURL: "https://src/ok"},
		{ID: 12, VideoID: 3, PlayPageURL: "https://src/enc"},
	}}
	f := &mapFetcher{pages: map[string]string{
		"https://src/vip": restricted,
		"https://src/ok":  playable,
		"https://src/enc": encrypted,
	}}
	sum, err := NewClassifyRestrictedWorker(st, f).Run(context.Background(), 0, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.MarkedRestricted != 1 || len(st.marks) != 1 || st.marks[0] != 10 {
		t.Fatalf("marks=%v sum=%+v", st.marks, sum)
	}
	if sum.PublicURLPresent != 1 || sum.NotMembership != 2 {
		t.Fatalf("%+v", sum)
	}
	if sum.NextAfterID != 12 {
		t.Fatalf("cursor=%d", sum.NextAfterID)
	}
}

func TestClassifyRestricted_dryRunDoesNotWrite(t *testing.T) {
	restricted := `<html><body>系统提示 您没有权限访问此数据，请升级会员</body></html>`
	st := &classifyMem{rows: []store.UnclassifiedUnavailableEpisode{
		{ID: 10, PlayPageURL: "https://src/vip"},
	}}
	f := &mapFetcher{pages: map[string]string{"https://src/vip": restricted}}
	sum, err := NewClassifyRestrictedWorker(st, f).Run(context.Background(), 0, 20, true)
	if err != nil {
		t.Fatal(err)
	}
	if sum.MarkedRestricted != 1 || len(st.marks) != 0 || sum.NextAfterID != 10 {
		t.Fatalf("marks=%v sum=%+v", st.marks, sum)
	}
}

func TestClassifyRestricted_fetchErrorDoesNotAdvancePastRow(t *testing.T) {
	st := &classifyMem{rows: []store.UnclassifiedUnavailableEpisode{
		{ID: 10, PlayPageURL: "https://src/ok"},
		{ID: 11, PlayPageURL: "https://src/bad"},
		{ID: 12, PlayPageURL: "https://src/later"},
	}}
	f := &mapFetcher{
		pages: map[string]string{"https://src/ok": `<html><body>plain</body></html>`},
		err:   map[string]error{"https://src/bad": errors.New("timeout")},
	}
	sum, err := NewClassifyRestrictedWorker(st, f).Run(context.Background(), 0, 20, false)
	if err == nil || !sum.StoppedOnError || sum.NextAfterID != 10 || sum.FetchFailed != 1 {
		t.Fatalf("err=%v sum=%+v", err, sum)
	}
	if len(st.marks) != 0 {
		t.Fatalf("marks=%v", st.marks)
	}
}

func TestClassifyRestricted_resumesAfterCursor(t *testing.T) {
	restricted := `<html><body>系统提示 您没有权限访问此数据，请升级会员</body></html>`
	st := &classifyMem{rows: []store.UnclassifiedUnavailableEpisode{
		{ID: 10, PlayPageURL: "https://src/old"},
		{ID: 11, PlayPageURL: "https://src/vip"},
	}}
	f := &mapFetcher{pages: map[string]string{
		"https://src/old": `<html></html>`,
		"https://src/vip": restricted,
	}}
	sum, err := NewClassifyRestrictedWorker(st, f).Run(context.Background(), 10, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Selected != 1 || sum.MarkedRestricted != 1 || st.marks[0] != 11 || sum.NextAfterID != 11 {
		t.Fatalf("marks=%v sum=%+v", st.marks, sum)
	}
}

func TestClassifyRestricted_limitBound(t *testing.T) {
	if err := ValidateClassifyRestrictedLimit(0); err == nil {
		t.Fatal("expected limit error")
	}
	if err := ValidateClassifyRestrictedLimit(21); err == nil {
		t.Fatal("expected limit error")
	}
	if err := ValidateClassifyRestrictedLimit(20); err != nil {
		t.Fatal(err)
	}
}
