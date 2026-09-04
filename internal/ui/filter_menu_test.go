package ui

import (
	"testing"

	"github.com/efolchmontiel/wsp-tui/internal/config"
	"github.com/efolchmontiel/wsp-tui/internal/store"
)

func TestVisibleChatFiltersDefaultHidesEstadosNovedades(t *testing.T) {
	vis := config.DefaultFilterVisibility()
	got := visibleChatFilters(vis)
	want := []store.ChatFilter{
		store.FilterAll, store.FilterFavorites, store.FilterGroups, store.FilterArchived,
	}
	if len(got) != len(want) {
		t.Fatalf("got %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("i=%d got %v want %v", i, got[i], want[i])
		}
	}
}

func TestVisibleChatFiltersNormalizeEmpty(t *testing.T) {
	got := visibleChatFilters(config.FilterVisibility{})
	if len(got) != 1 || got[0] != store.FilterAll {
		t.Fatalf("%#v", got)
	}
}
