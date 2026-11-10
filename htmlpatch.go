package main

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// headOp is one safe, whitelisted edit to a page's <head>.
type headOp struct {
	Op    string `json:"op"` // title | meta | canonical | lang | jsonld | remove_noindex
	Key   string `json:"key"`
	Value string `json:"value"`
}

var (
	reHeadClose  = regexp.MustCompile(`(?i)</head\s*>`)
	reTitleTag   = regexp.MustCompile(`(?is)<title[^>]*>.*?</title>`)
	reCanonTag   = regexp.MustCompile(`(?is)<link\s[^>]*\brel\s*=\s*["']canonical["'][^>]*>`)
	reHTMLTag    = regexp.MustCompile(`(?is)<html\b[^>]*>`)
	reLangAttr   = regexp.MustCompile(`(?is)\slang\s*=\s*["'][^"']*["']`)
	reRobotsMeta = regexp.MustCompile(`(?is)<meta\s[^>]*\bname\s*=\s*["']robots["'][^>]*>`)
	reJSONKey    = regexp.MustCompile(`^[a-z0-9-]{1,20}$`)
	reLangVal    = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})?$`)
)

var okMetaNames = map[string]bool{"description": true, "viewport": true, "twitter:card": true}

func metaRe(attr, key string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)<meta\s[^>]*\b` + attr + `\s*=\s*["']` + regexp.QuoteMeta(key) + `["'][^>]*>`)
}

// replaceFirst swaps the first match of re for repl, or inserts repl before </head> when there is none.
func replaceOrInsert(doc string, re *regexp.Regexp, repl string) (string, error) {
	if loc := re.FindStringIndex(doc); loc != nil {
		return doc[:loc[0]] + repl + doc[loc[1]:], nil
	}
	loc := reHeadClose.FindStringIndex(doc)
	if loc == nil {
		return "", fmt.Errorf("the page has no </head>, so nothing was changed")
	}
	return doc[:loc[0]] + "  " + repl + "\n" + doc[loc[0]:], nil
}

// patchHead applies ops to an HTML document and returns the new document and a list of what changed.
func patchHead(doc string, ops []headOp) (string, []string, error) {
	var changed []string
	for _, o := range ops {
		before := doc
		var err error
		switch o.Op {
		case "title":
			v := strings.TrimSpace(o.Value)
			if v == "" || len([]rune(v)) > 200 {
				return "", nil, fmt.Errorf("the title must be 1-200 characters")
			}
			doc, err = replaceOrInsert(doc, reTitleTag, "<title>"+html.EscapeString(v)+"</title>")
			changed = append(changed, "page title")
		case "meta":
			k := o.Key
			prop := strings.HasPrefix(k, "og:")
			if !okMetaNames[k] && !prop {
				return "", nil, fmt.Errorf("meta tag %q is not allowed", k)
			}
			attr := "name"
			if prop {
				attr = "property"
			}
			v := strings.TrimSpace(o.Value)
			if v == "" || len([]rune(v)) > 400 {
				return "", nil, fmt.Errorf("meta %s must be 1-400 characters", k)
			}
			doc, err = replaceOrInsert(doc, metaRe(attr, k), `<meta `+attr+`="`+k+`" content="`+html.EscapeString(v)+`">`)
			changed = append(changed, k)
		case "canonical":
			v := strings.TrimSpace(o.Value)
			if !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
				return "", nil, fmt.Errorf("canonical must be a full URL")
			}
			doc, err = replaceOrInsert(doc, reCanonTag, `<link rel="canonical" href="`+html.EscapeString(v)+`">`)
			changed = append(changed, "canonical link")
		case "lang":
			if !reLangVal.MatchString(o.Value) {
				return "", nil, fmt.Errorf("language code %q looks wrong (use e.g. en, am, ar, fr)", o.Value)
			}
			loc := reHTMLTag.FindStringIndex(doc)
			if loc == nil {
				return "", nil, fmt.Errorf("the page has no <html> tag")
			}
			tag := doc[loc[0]:loc[1]]
			if reLangAttr.MatchString(tag) {
				tag = reLangAttr.ReplaceAllString(tag, ` lang="`+o.Value+`"`)
			} else {
				tag = strings.TrimSuffix(tag, ">") + ` lang="` + o.Value + `">`
			}
			doc = doc[:loc[0]] + tag + doc[loc[1]:]
			changed = append(changed, "html lang")
		case "jsonld":
			if !reJSONKey.MatchString(o.Key) || !json.Valid([]byte(o.Value)) || len(o.Value) > 60000 {
				return "", nil, fmt.Errorf("structured data is not valid JSON")
			}
			var buf strings.Builder
			compact := new(strings.Builder)
			if err := jsonCompact(compact, o.Value); err != nil {
				return "", nil, err
			}
			buf.WriteString(`<script type="application/ld+json" data-blitz="` + o.Key + `">` + strings.ReplaceAll(compact.String(), "</", `<\/`) + `</script>`)
			re := regexp.MustCompile(`(?is)<script[^>]*data-blitz\s*=\s*["']` + regexp.QuoteMeta(o.Key) + `["'][^>]*>.*?</script>`)
			doc, err = replaceOrInsert(doc, re, buf.String())
			changed = append(changed, "structured data ("+o.Key+")")
		case "remove_noindex":
			doc = reRobotsMeta.ReplaceAllStringFunc(doc, func(t string) string {
				if strings.Contains(strings.ToLower(t), "noindex") {
					return ""
				}
				return t
			})
			changed = append(changed, "removed noindex")
		default:
			return "", nil, fmt.Errorf("unknown change %q", o.Op)
		}
		if err != nil {
			return "", nil, err
		}
		if doc == before && len(changed) > 0 {
			changed = changed[:len(changed)-1]
		}
	}
	return doc, changed, nil
}

func jsonCompact(dst *strings.Builder, src string) error {
	var v any
	if err := json.Unmarshal([]byte(src), &v); err != nil {
		return fmt.Errorf("structured data is not valid JSON")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dst.Write(b)
	return nil
}
