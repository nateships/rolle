//go:build !production

package main

// Development builds (wails3 dev, go run) expose in-app dev tools.
const devMode = true

// DevReset wipes the workspace, secrets, cache, and profiles so onboarding runs again.
func (r *RolleService) DevReset() error {
	err := r.svc.ResetAll()
	r.changed()
	return err
}

// DevReplayOnboarding shows the walkthrough again without removing sessions.
func (r *RolleService) DevReplayOnboarding() error {
	err := r.svc.ReplayOnboarding()
	r.changed()
	return err
}
