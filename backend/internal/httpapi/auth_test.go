package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestLoginCreatesPKCEStateAndRedirects(t *testing.T) {
	auth := Authenticator{
		ClientID:         "client-id",
		AuthorizationURL: "https://auth.example.test/oauth2/authorize",
		RedirectURL:      "https://beast.example.test/api/auth/callback",
		SecureCookies:    true,
	}
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/login", nil)
	recorder := httptest.NewRecorder()
	if err := auth.Login(e.NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusFound {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	if query.Get("client_id") != auth.ClientID || query.Get("redirect_uri") != auth.RedirectURL || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected authorization query: %v", query)
	}
	if query.Get("state") == "" || query.Get("code_challenge") == "" {
		t.Fatalf("authorization query does not contain PKCE parameters: %v", query)
	}
	stateCookie := responseCookie(t, recorder, auth.loginStateCookieName())
	parts := strings.Split(stateCookie.Value, ".")
	if len(parts) != 2 || parts[0] != query.Get("state") {
		t.Fatalf("state cookie does not match authorization state: %q", stateCookie.Value)
	}
	if !stateCookie.HttpOnly || !stateCookie.Secure || stateCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("state cookie is not protected: %#v", stateCookie)
	}
	if query.Get("code_challenge") != codeChallenge(parts[1]) {
		t.Fatal("PKCE challenge does not match verifier")
	}
}

func TestCallbackExchangesCodeWithBasicAuthAndSetsSessionCookie(t *testing.T) {
	var tokenBasicAuth bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			username, password, ok := r.BasicAuth()
			tokenBasicAuth = ok && username == "client-id" && password == "client-secret"
			if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code_verifier") == "" {
				http.Error(w, "invalid token request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","token_type":"Bearer","expires_in":3600}`))
		case "/introspect":
			if err := r.ParseForm(); err != nil || r.Form.Get("token") != "access-token" {
				http.Error(w, "invalid introspection request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":true,"sub":"user-123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	auth := Authenticator{
		ClientID:         "client-id",
		ClientSecret:     "client-secret",
		AuthorizationURL: provider.URL + "/authorize",
		TokenURL:         provider.URL + "/token",
		Introspection:    provider.URL + "/introspect",
		RedirectURL:      "https://beast.example.test/api/auth/callback",
		FrontendURL:      "https://beast.example.test/",
		SecureCookies:    true,
		HTTPClient:       provider.Client(),
	}
	e := echo.New()
	loginRecorder := httptest.NewRecorder()
	if err := auth.Login(e.NewContext(httptest.NewRequest(http.MethodGet, "/api/auth/login", nil), loginRecorder)); err != nil {
		t.Fatal(err)
	}
	stateCookie := responseCookie(t, loginRecorder, auth.loginStateCookieName())
	parts := strings.Split(stateCookie.Value, ".")
	callbackURL := "/api/auth/callback?code=authorization-code&state=" + url.QueryEscape(parts[0])
	callbackRequest := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	callbackRequest.AddCookie(stateCookie)
	callbackRecorder := httptest.NewRecorder()
	if err := auth.Callback(e.NewContext(callbackRequest, callbackRecorder)); err != nil {
		t.Fatal(err)
	}
	if !tokenBasicAuth {
		t.Fatal("token exchange did not use HTTP Basic authentication")
	}
	if callbackRecorder.Code != http.StatusFound || callbackRecorder.Header().Get("Location") != auth.FrontendURL {
		t.Fatalf("unexpected callback response: %d %q", callbackRecorder.Code, callbackRecorder.Header().Get("Location"))
	}
	sessionCookie := responseCookie(t, callbackRecorder, auth.sessionCookieName())
	if sessionCookie.Value != "access-token" || sessionCookie.MaxAge != 3600 || !sessionCookie.HttpOnly || !sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie: %#v", sessionCookie)
	}
	if clearedState := responseCookie(t, callbackRecorder, auth.loginStateCookieName()); clearedState.MaxAge >= 0 || clearedState.Value != "" {
		t.Fatalf("state cookie was not cleared: %#v", clearedState)
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	sessionRequest.AddCookie(sessionCookie)
	subject, err := auth.Subject(t.Context(), sessionRequest)
	if err != nil || subject != "user-123" {
		t.Fatalf("session cookie was not accepted: subject=%q err=%v", subject, err)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	auth := Authenticator{ClientID: "client-id", ClientSecret: "client-secret", TokenURL: "https://auth.example.test/token", RedirectURL: "https://beast.example.test/api/auth/callback"}
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=code&state=returned", nil)
	request.AddCookie(&http.Cookie{Name: auth.loginStateCookieName(), Value: "stored.verifier"})
	recorder := httptest.NewRecorder()
	err := auth.Callback(e.NewContext(request, recorder))
	if err == nil {
		t.Fatalf("state mismatch was accepted: err=%v status=%d", err, recorder.Code)
	}
}

func TestNativeCallbackReturnsTokenInFragment(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"native-token","token_type":"Bearer","expires_in":3600}`))
		case "/introspect":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":true,"sub":"user-123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	auth := Authenticator{
		ClientID:          "client-id",
		ClientSecret:      "client-secret",
		AuthorizationURL:  provider.URL + "/authorize",
		TokenURL:          provider.URL + "/token",
		Introspection:     provider.URL + "/introspect",
		RedirectURL:       "https://beast.example.test/api/auth/callback",
		NativeRedirectURL: "dev.walnuts.beast://oauth2redirect",
		SecureCookies:     true,
		HTTPClient:        provider.Client(),
	}
	e := echo.New()
	loginRecorder := httptest.NewRecorder()
	if err := auth.NativeLogin(e.NewContext(httptest.NewRequest(http.MethodGet, "/api/auth/mobile/login?state=native-state-123456", nil), loginRecorder)); err != nil {
		t.Fatal(err)
	}
	stateCookie := responseCookie(t, loginRecorder, auth.loginStateCookieName())
	nativeCookie := responseCookie(t, loginRecorder, auth.nativeStateCookieName())
	state, _, ok := strings.Cut(stateCookie.Value, ".")
	if !ok {
		t.Fatalf("invalid state cookie: %q", stateCookie.Value)
	}
	callbackRequest := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=authorization-code&state="+url.QueryEscape(state), nil)
	callbackRequest.AddCookie(stateCookie)
	callbackRequest.AddCookie(nativeCookie)
	callbackRecorder := httptest.NewRecorder()
	if err := auth.Callback(e.NewContext(callbackRequest, callbackRecorder)); err != nil {
		t.Fatal(err)
	}
	location := callbackRecorder.Header().Get("Location")
	if callbackRecorder.Code != http.StatusFound || !strings.HasPrefix(location, "dev.walnuts.beast://oauth2redirect#") {
		t.Fatalf("unexpected native callback: status=%d location=%q", callbackRecorder.Code, location)
	}
	handoff, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(handoff.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.Get("access_token") != "native-token" || fragment.Get("state") != "native-state-123456" || fragment.Get("expires_in") != "3600" {
		t.Fatalf("unexpected native handoff fragment: %v", fragment)
	}
	for _, cookie := range callbackRecorder.Result().Cookies() {
		if cookie.Name == auth.sessionCookieName() && cookie.Value != "" {
			t.Fatal("native callback must not create a browser session cookie")
		}
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	auth := Authenticator{SecureCookies: true}
	e := echo.New()
	recorder := httptest.NewRecorder()
	if err := auth.Logout(e.NewContext(httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil), recorder)); err != nil {
		t.Fatal(err)
	}
	cookie := responseCookie(t, recorder, auth.sessionCookieName())
	if cookie.MaxAge >= 0 || cookie.Value != "" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie was not cleared securely: %#v", cookie)
	}
}

func responseCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, header := range recorder.Result().Cookies() {
		if header.Name == name {
			return header
		}
	}
	t.Fatalf("cookie %q was not returned", name)
	return nil
}
