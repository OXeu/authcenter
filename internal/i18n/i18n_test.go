package i18n

import (
	"testing"
)

func TestParseAcceptLanguageOrdersByQuality(t *testing.T) {
	t.Parallel()
	got := ParseAcceptLanguage("fr;q=0.5, zh-CN;q=0.9, en;q=0.8, *;q=1")
	want := []string{"zh-CN", "en", "fr"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestResolveFallsBackToBaseLanguage(t *testing.T) {
	t.Parallel()
	catalog, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	enabled := []string{"en", "zh-CN"}
	if got := catalog.Resolve(enabled, "zh-Hant-TW"); got != "en" {
		t.Fatalf("unknown language resolved to %q", got)
	}
	if got := catalog.Resolve(enabled, "zh"); got != "en" {
		t.Fatalf("bare zh resolved to %q", got)
	}
	if got := catalog.Resolve(enabled, "zh-CN"); got != "zh-CN" {
		t.Fatalf("zh-CN resolved to %q", got)
	}
	if got := catalog.Resolve([]string{"en"}, "zh-CN"); got != Default {
		t.Fatalf("disabled language resolved to %q", got)
	}
}

func TestEveryLocaleHasEveryKey(t *testing.T) {
	t.Parallel()
	catalog, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	reference := catalog.Messages(Default)
	for _, tag := range catalog.tags {
		table := catalog.Messages(tag)
		for id := range table {
			if _, ok := reference[id]; !ok {
				t.Fatalf("%s defines %q which %s lacks", tag, id, Default)
			}
		}
		if table["language"] == "" {
			t.Fatalf("%s has no language label", tag)
		}
	}
}

func TestResolveUsesFirstEnabledLanguage(t *testing.T) {
	catalog, err := Shared()
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Resolve([]string{"zh-CN", "en"}); got != "zh-CN" {
		t.Fatal(got)
	}
	if got := catalog.Resolve([]string{"zh-CN"}, "fr"); got != "zh-CN" {
		t.Fatal(got)
	}
}
