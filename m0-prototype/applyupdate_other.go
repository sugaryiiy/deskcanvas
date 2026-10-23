//go:build !darwin

package main

import (
	"fmt"
	"log"
)

// Non-macOS stubs for the updater's platform hooks: update *checking*
// is portable, but installing an update means mounting a DMG and
// swapping an .app bundle, which only exists on macOS.

func extractAppFromDMG(dmgPath, stagedDir string) (string, error) {
	return "", fmt.Errorf("DMG extraction is macOS-only")
}

func launchApplyHelper(stagedApp string) error {
	log.Printf("deskcanvas [stub]: would apply staged update %s (no-op off macOS)", stagedApp)
	return fmt.Errorf("self-update install is macOS-only")
}

func runApplyUpdateIfRequested() bool { return false }

func cleanupPreviousBundle() {}
