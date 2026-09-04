package ui

import "testing"

func TestEmojiGridMove(t *testing.T) {
	// 23 items, 10 cols → 3 rows
	n, cols := 23, 10
	if got := emojiGridMove(0, n, cols, 0, 1); got != 1 {
		t.Fatalf("right from 0: %d", got)
	}
	if got := emojiGridMove(9, n, cols, 0, 1); got != 9 {
		t.Fatalf("right at edge stays: %d", got)
	}
	if got := emojiGridMove(5, n, cols, 1, 0); got != 15 {
		t.Fatalf("down: %d", got)
	}
	if got := emojiGridMove(22, n, cols, 1, 0); got != 22 {
		t.Fatalf("down past last: %d", got)
	}
	if got := clampEmojiIdx(99, n); got != 22 {
		t.Fatalf("clamp: %d", got)
	}
}

func TestEmojiCategoriesNonEmpty(t *testing.T) {
	if len(emojiCategories) == 0 {
		t.Fatal("no categories")
	}
	for _, c := range emojiCategories {
		if c.Name == "" || len(c.Items) == 0 {
			t.Fatalf("bad category %+v", c)
		}
	}
}
