// Package owner gives files written while running as root back to the user
// they belong to.
//
// The client restarts itself with administrative rights to create the TUN
// device, so everything it writes then belongs to root. The user's own
// settings and the extracted core must stay theirs: without this, the next
// run without rights could not save anything or replace the core.
package owner

// Keep gives a file or directory under dir to whoever owns dir. It does
// nothing unless this process is root.
func Keep(dir, path string) { keep(dir, path) }
