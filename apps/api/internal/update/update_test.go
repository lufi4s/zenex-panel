package update

import (
	"context"
	"testing"
)

type fakeHelper map[string]string

func (f fakeHelper) Output(_ context.Context, op string, _ map[string]string) (string, error) {
	return f[op], nil
}
func (f fakeHelper) Do(context.Context, string, map[string]string) (string, error) {
	return "", nil
}

func TestStatusListsTheChangesOfANewVersion(t *testing.T) {
	st, err := New(fakeHelper{
		"panel.version":       "aaa1111",
		"panel.latest":        "bbb2222",
		"panel.changes":       "bbb2222\tPage cache\nccc3333\tFaster mail\n\nbad line",
		"panel.update-status": `{"state":"idle","log":""}`,
	}).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.UpdateAvailable || len(st.Changes) != 2 || st.Changes[0].Title != "Page cache" || st.Changes[1].ID != "ccc3333" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestStatusHasNoChangesWhenUpToDate(t *testing.T) {
	st, err := New(fakeHelper{
		"panel.version":       "aaa1111",
		"panel.latest":        "aaa1111",
		"panel.update-status": `{"state":"idle","log":""}`,
	}).Status(context.Background())
	if err != nil || st.UpdateAvailable || st.Changes == nil || len(st.Changes) != 0 {
		t.Fatalf("unexpected: %+v %v", st, err)
	}
}
