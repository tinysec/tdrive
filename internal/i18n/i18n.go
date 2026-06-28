package i18n

import (
	"context"
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localeFiles embed.FS

// defaultLanguage is used when a requested language is unknown or a key is missing.
const defaultLanguage string = "en"

// Translator looks up localized strings for a single language, falling back to
// the default language and finally to the key itself when nothing matches.
type Translator struct {
	lang     string
	messages map[string]string
	fallback map[string]string
}

// Lang reports the active language code of this translator ("zh" or "en").
func (translator *Translator) Lang() string {

	return translator.lang
}

// T returns the localized string for key, or the key itself if it is undefined.
func (translator *Translator) T(key string) string {

	var value string
	var ok bool

	value, ok = translator.messages[key]
	if ok {
		return value
	}

	value, ok = translator.fallback[key]
	if ok {
		return value
	}

	return key
}

// Tf returns the localized string for key formatted with the given arguments.
func (translator *Translator) Tf(key string, args ...any) string {

	return fmt.Sprintf(translator.T(key), args...)
}

// Bundle holds the loaded message tables for every supported language.
type Bundle struct {
	tables map[string]map[string]string
}

// supportedLanguages lists the language codes the product ships with.
func supportedLanguages() []string {

	return []string{"en", "zh"}
}

// NewBundle loads all embedded locale files into memory.
func NewBundle() (*Bundle, error) {

	var bundle Bundle

	bundle.tables = make(map[string]map[string]string)

	var languages []string = supportedLanguages()

	for _, lang := range languages {

		var path string = fmt.Sprintf("locales/%s.yaml", lang)

		var raw []byte
		var err error

		raw, err = localeFiles.ReadFile(path)
		if nil != err {
			return nil, fmt.Errorf("read locale %q: %w", path, err)
		}

		var table map[string]string = make(map[string]string)

		err = yaml.Unmarshal(raw, &table)
		if nil != err {
			return nil, fmt.Errorf("parse locale %q: %w", path, err)
		}

		bundle.tables[lang] = table
	}

	return &bundle, nil
}

// Supports reports whether the bundle has a table for the given language.
func (bundle *Bundle) Supports(lang string) bool {

	var _, ok = bundle.tables[lang]

	return ok
}

// Translator builds a Translator for the requested language, defaulting when unknown.
func (bundle *Bundle) Translator(lang string) *Translator {

	if false == bundle.Supports(lang) {
		lang = defaultLanguage
	}

	var translator Translator

	translator.lang = lang
	translator.messages = bundle.tables[lang]
	translator.fallback = bundle.tables[defaultLanguage]

	return &translator
}

// contextKey is a private type so translator values cannot collide in the context.
type contextKey struct{}

var translatorKey contextKey = contextKey{}

// WithTranslator returns a context carrying the given translator.
func WithTranslator(ctx context.Context, translator *Translator) context.Context {

	return context.WithValue(ctx, translatorKey, translator)
}

// FromContext returns the translator bound to ctx, or a safe empty translator.
func FromContext(ctx context.Context) *Translator {

	var value any = ctx.Value(translatorKey)

	var translator *Translator
	var ok bool

	translator, ok = value.(*Translator)
	if ok && nil != translator {
		return translator
	}

	// Defensive fallback: return a translator that echoes keys so templates never panic.
	var empty Translator

	empty.lang = defaultLanguage
	empty.messages = map[string]string{}
	empty.fallback = map[string]string{}

	return &empty
}

// T is a template-friendly helper that translates key using the context translator.
func T(ctx context.Context, key string) string {

	return FromContext(ctx).T(key)
}
