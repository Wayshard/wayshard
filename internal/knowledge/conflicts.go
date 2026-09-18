package knowledge

import (
	"strings"
	"unicode"
)

// ConflictWarning is a heuristic possible contradiction. It never blocks
// project open and never assigns document authority.
type ConflictWarning struct {
	Domain   AuthorityDomain
	PathA    string
	PathB    string
	ExcerptA string
	ExcerptB string
	Reason   string
}

type directive struct {
	path     string
	domain   AuthorityDomain
	polarity int
	tokens   []string
	tokenSet map[string]struct{}
	sentence string
}

var stopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "to": {}, "of": {}, "and": {}, "or": {},
	"in": {}, "on": {}, "for": {}, "is": {}, "are": {}, "be": {}, "by": {},
	"with": {}, "from": {}, "that": {}, "this": {}, "as": {}, "at": {},
	"it": {}, "if": {}, "not": {}, "do": {}, "does": {}, "must": {},
	"shall": {}, "should": {}, "always": {}, "never": {}, "cannot": {},
	"can": {}, "may": {}, "required": {}, "forbidden": {}, "don't": {},
	"dont": {}, "no": {}, "yes": {}, "we": {}, "you": {}, "use": {},
	"using": {}, "used": {}, "into": {}, "than": {}, "then": {},
}

var negPhrases = []string{
	"must not", "shall not", "do not", "don't", "never", "cannot",
	"can not", "forbidden", "disallowed", "is not allowed", "are not allowed",
}

var posPhrases = []string{
	"must", "always", "shall", "required", "is required",
}

// DetectConflicts finds same-domain directives with opposite polarity and
// overlapping content words. Warnings are advisory only.
func DetectConflicts(docs []Document) []ConflictWarning {
	var dirs []directive
	for _, doc := range docs {
		if doc.Content == "" {
			continue
		}
		for _, sent := range splitSentences(doc.Content) {
			pol, ok := polarity(sent)
			if !ok {
				continue
			}
			toks := contentTokens(sent)
			if len(toks) < 2 {
				continue
			}
			set := map[string]struct{}{}
			for _, t := range toks {
				set[t] = struct{}{}
			}
			dirs = append(dirs, directive{
				path:     doc.Path,
				domain:   doc.AuthorityDomain,
				polarity: pol,
				tokens:   toks,
				tokenSet: set,
				sentence: clip(sent, 180),
			})
		}
	}
	var out []ConflictWarning
	seen := map[string]struct{}{}
	for i := 0; i < len(dirs); i++ {
		for j := i + 1; j < len(dirs); j++ {
			a, b := dirs[i], dirs[j]
			if a.path == b.path || a.domain != b.domain || a.domain == "" {
				continue
			}
			if a.polarity == 0 || a.polarity != -b.polarity {
				continue
			}
			shared, jac := jaccard(a, b)
			if shared < 2 || jac < 0.34 {
				continue
			}
			key := a.domain.String() + "\x00" + orderedPair(a.path, b.path)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ConflictWarning{
				Domain:   a.domain,
				PathA:    a.path,
				PathB:    b.path,
				ExcerptA: a.sentence,
				ExcerptB: b.sentence,
				Reason:   "same authority domain with opposite directives",
			})
		}
	}
	return out
}

func polarity(sent string) (int, bool) {
	s := strings.ToLower(sent)
	for _, p := range negPhrases {
		if strings.Contains(s, p) {
			return -1, true
		}
	}
	for _, p := range posPhrases {
		if strings.Contains(s, p) {
			return 1, true
		}
	}
	return 0, false
}

func splitSentences(content string) []string {
	content = stripFences(content)
	var out []string
	var b strings.Builder
	flush := func() {
		s := strings.TrimSpace(b.String())
		b.Reset()
		if len(s) >= 12 {
			out = append(out, s)
		}
	}
	for _, r := range content {
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' || r == '\n' {
			flush()
		}
	}
	flush()
	return out
}

func stripFences(s string) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func contentTokens(sent string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		t := strings.ToLower(b.String())
		b.Reset()
		if t == "" {
			return
		}
		if _, stop := stopwords[t]; stop {
			return
		}
		if len(t) < 3 {
			return
		}
		out = append(out, t)
	}
	for _, r := range sent {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return uniqueStable(out)
}

func jaccard(a, b directive) (shared int, score float64) {
	for t := range a.tokenSet {
		if _, ok := b.tokenSet[t]; ok {
			shared++
		}
	}
	union := len(a.tokenSet) + len(b.tokenSet) - shared
	if union == 0 {
		return 0, 0
	}
	return shared, float64(shared) / float64(union)
}

func orderedPair(a, b string) string {
	if a < b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
