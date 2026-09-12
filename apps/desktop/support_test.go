package main

import "testing"

func TestSupportURL(t *testing.T) {
	got := supportURL("0.1.1", "darwin", "arm64")
	want := "https://github.com/nateships/rolle/issues/new?platform=darwin+arm64&template=bug.yml&version=0.1.1"
	if got != want {
		t.Fatalf("supportURL = %s, want %s", got, want)
	}
	if got := supportURL("", "linux", "amd64"); got != "https://github.com/nateships/rolle/issues/new?platform=linux+amd64&template=bug.yml&version=dev" {
		t.Fatalf("dev supportURL = %s", got)
	}
}
