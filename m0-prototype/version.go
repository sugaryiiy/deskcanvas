package main

// Version is the build version, injected by CI at release time via
//
//	wails build -ldflags "-X main.Version=v0.1.1-m0"
//
// Local/dev builds keep "dev", and the updater stays silent for them:
// auto-update only ever runs between two real release builds.
var Version = "dev"
