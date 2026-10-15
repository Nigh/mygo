//go:build linux && (amd64 || arm64)

package gl

// Info identifies the current context for diagnostic captures.
func (r *Renderer) Info() string {
	return goString(glGetString(glVersion)) + ", " + goString(glGetString(glRenderer))
}
