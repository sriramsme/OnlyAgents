package memory

import (
	"fmt"
	"strings"
)

type Context struct {
	Results []Result // all sources merged, ranked by score
}

func (mc *Context) Render() string {
	if mc == nil {
		return ""
	}

	var b strings.Builder

	if len(mc.Results) > 0 {
		for _, r := range mc.Results {
			source := strings.TrimSpace(r.SourceName)

			if source != "" {
				fmt.Fprintf(&b, "- (Source: %s) %s\n", source, r.Content)
			} else {
				fmt.Fprintf(&b, "- %s\n", r.Content)
			}
		}
	}

	return strings.TrimSpace(b.String())
}
