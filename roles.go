package viewbook

import "strings"

// The roles a relation can carry. A model is a document people write by hand,
// so these are the words the book understands and nothing else is guessed at.
const (
	roleStateOf     = "State of"
	roleLivesIn     = "Lives in"
	roleReachedFrom = "Reached from"
)

var rolesKnown = []string{roleStateOf, roleLivesIn, roleReachedFrom}

// sameRole is whether a relation says the role that was meant. Case and the
// punctuation between the words are the writer's, not the model's: "state_of"
// and "State of" are one word to everybody but a string comparison, and a
// relation silently ignored over a capital letter reads as a missing render
// rather than as a typo.
func sameRole(said, meant string) bool {
	return roleKey(said) == roleKey(meant)
}

// knownRole is whether a role is one of the roles this book acts on, which is
// what makes an unrecognised one worth saying out loud.
func knownRole(role string) bool {
	for _, known := range rolesKnown {
		if sameRole(role, known) {
			return true
		}
	}
	return false
}

// roleKey is a role with everything a writer may spell differently taken out:
// the letters, lowercase, with spaces, underscores and hyphens gone.
func roleKey(role string) string {
	var key strings.Builder
	for _, letter := range strings.ToLower(role) {
		if letter >= 'a' && letter <= 'z' {
			key.WriteRune(letter)
		}
	}
	return key.String()
}
