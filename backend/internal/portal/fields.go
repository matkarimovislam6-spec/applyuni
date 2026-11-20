package portal

import (
	"fmt"
	"strings"
)

// BuildFields flattens what we know about the student into the "{{...}}"
// values flows use. Profile values become profile.<key>; nested lists are
// skipped. Names are split into first/last the way most portals ask.
func BuildFields(studentName, email, portalUser, portalPassword string, profile map[string]any, program, degree string) map[string]string {
	f := map[string]string{
		"student.full_name": strings.TrimSpace(studentName),
		"student.email":     email,
		"portal.username":   portalUser,
		"portal.password":   portalPassword,
		"program.name":      program,
		"program.degree":    degree,
	}
	parts := strings.Fields(studentName)
	if len(parts) > 0 {
		f["student.first_name"] = parts[0]
		f["student.last_name"] = strings.Join(parts[1:], " ")
	}
	for k, v := range profile {
		switch x := v.(type) {
		case string:
			f["profile."+k] = x
		case float64:
			f["profile."+k] = strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.4f", x), "0000"), ".")
		case bool:
			f["profile."+k] = fmt.Sprintf("%t", x)
		}
	}
	return f
}
