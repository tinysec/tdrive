package meta

// Name is the single source of truth for the product's short identifier.
// Everything that embeds the product name — session cookie, the default FTP user,
// the OS temp upload directory, and the UI title — derives from this constant, so
// renaming the product is a one-line change here.
const Name string = "tdrive"

// SessionCookie is the cookie name used to carry the login session token.
func SessionCookie() string {

	return Name + "_session"
}

// LanguageCookie is the cookie name used to remember the chosen UI language.
func LanguageCookie() string {

	return Name + "_lang"
}
