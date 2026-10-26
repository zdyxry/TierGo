package locale

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func hasHan(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
func TestEnglishCatalogCoverageAndFormats(t *testing.T) {
	// Source-audit every user-facing Chinese literal. Fail new additions lacking
	// translations instead of quietly shipping Chinese in the English UI.
	formats := regexp.MustCompile(`%[-+# 0]*(?:\d+)?(?:\.\d+)?[a-zA-Z%]`)
	for source, translated := range english {
		if strings.TrimSpace(translated) == "" || hasHan(translated) {
			t.Errorf("untranslated %q: %q", source, translated)
		}
		if !reflect.DeepEqual(formats.FindAllString(source, -1), formats.FindAllString(translated, -1)) {
			t.Errorf("format directives changed: %q => %q", source, translated)
		}
	}
	root := filepath.Join("..", "..")
	for _, dir := range []string{"internal/app", "internal/easytier", "."} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, dir, entry.Name()), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				lit, ok := node.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil || !hasHan(value) {
					return true
				}
				if value == "Language / 语言" || value == "简体中文" {
					return true
				}
				if _, ok := english[value]; !ok {
					t.Errorf("missing translation in %s: %q", entry.Name(), value)
				}
				return true
			})
		}
	}
}
func TestLanguageNormalizationAndFallback(t *testing.T) {
	old := Language()
	defer Set(old)
	Set("en-US")
	if T("设置") != "Settings" {
		t.Fatal("English selection failed")
	}
	Set("zh-CN")
	if T("设置") != "设置" {
		t.Fatal("Chinese selection failed")
	}
	if In("en", "user-defined-network") != "user-defined-network" {
		t.Fatal("user text changed")
	}
}
