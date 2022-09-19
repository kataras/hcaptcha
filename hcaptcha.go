package hcaptcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

var (
	// ResponseContextKey is the default request's context key that response of a hcaptcha request is kept.
	ResponseContextKey interface{} = "hcaptcha"
	// DefaultFailureHandler is the default HTTP handler that is fired on hcaptcha failures. See `Client.FailureHandler`.
	DefaultFailureHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
	})

	// PostMaxMemory is the max memory for a form, defaults to 32MB
	PostMaxMemory int64 = 32 << 20
)

// Client represents the hcaptcha client.
// It contains the underline HTTPClient which can be modified before API calls.
type Client struct {
	HTTPClient *http.Client

	// FailureHandler if specified, fired when user does not complete hcaptcha successfully.
	// Failure and error codes information are kept as `Response` type
	// at the Request's Context key of "hcaptcha".
	//
	// Defaults to a handler that writes a status code of 429 (Too Many Requests)
	// and without additional information.
	FailureHandler http.Handler

	// Optional checks for siteverify
	// The user's IP address.
	RemoteIP string
	// The sitekey you expect to see.
	SiteKey string

	secret string
}

// Response is the hcaptcha JSON response.
type Response struct {
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
	Success     bool     `json:"success"`
	Credit      bool     `json:"credit,omitempty"`
}

// New accepts a hpcatcha secret key and returns a new hcaptcha HTTP Client.
//
// Instructions at: https://docs.hcaptcha.com/.
//
// See its `Handler` and `SiteVerify` for details.
func New(secret string) *Client {
	return &Client{
		HTTPClient:     http.DefaultClient,
		FailureHandler: DefaultFailureHandler,
		secret:         secret,
	}
}

// Handler is the HTTP route middleware featured hcaptcha validation.
// It calls the `SiteVerify` method and fires the "next" when user completed the hcaptcha successfully,
//
//	otherwise it calls the Client's `FailureHandler`.
//
// The hcaptcha's `Response` (which contains any `ErrorCodes`)
// is saved on the Request's Context (see `GetResponseFromContext`).
func (c *Client) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := c.SiteVerify(r)
		r = r.WithContext(context.WithValue(r.Context(), ResponseContextKey, v))
		if v.Success {
			next.ServeHTTP(w, r)
			return
		}

		if c.FailureHandler != nil {
			c.FailureHandler.ServeHTTP(w, r)
		}
	})
}

// HandlerFunc same as `Handler` but it accepts and returns a type of `http.HandlerFunc` instead.
func (c *Client) HandlerFunc(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return c.Handler(http.HandlerFunc(next)).ServeHTTP
}

// responseFormValue = "h-captcha-response"
const apiURL = "https://hcaptcha.com/siteverify"

// SiteVerify accepts a "r" Request and a secret key (https://dashboard.hcaptcha.com/settings).
// It returns the hcaptcha's `Response`.
// The `response.Success` reports whether the validation passed.
// Any errors are passed through the `response.ErrorCodes` field.
func (c *Client) SiteVerify(r *http.Request) (response Response) {
	generatedResponseID, err := getFormValue(r, "h-captcha-response")
	if err != nil {
		response.ErrorCodes = append(response.ErrorCodes, err.Error())
		return
	}

	if generatedResponseID == "" {
		response.ErrorCodes = append(response.ErrorCodes,
			"form[h-captcha-response] is empty")
		return
	}

	// Call VerifyToken for verification after extracting token
	// Check token before call to maintain backwards compatibility
	return c.VerifyToken(generatedResponseID)
}

// VerifyToken accepts a token and a secret key (https://dashboard.hcaptcha.com/settings).
// It returns the hcaptcha's `Response`.
// The `response.Success` reports whether the validation passed.
// Any errors are passed through the `response.ErrorCodes` field.
// Same as SiteVerify except token is provided by caller instead of being extracted from HTTP request
func (c *Client) VerifyToken(tkn string) (response Response) {
	if tkn == "" {
		response.ErrorCodes = append(response.ErrorCodes, errors.New("tkn is empty").Error())
		return
	}

	values := url.Values{
		"secret":   {c.secret},
		"response": {tkn},
	}

	// Add remoteIP if set
	if c.RemoteIP != "" {
		values.Add("remoteip", c.RemoteIP)
	}

	// Add sitekey if set
	if c.SiteKey != "" {
		values.Add("sitekey", c.SiteKey)
	}

	resp, err := c.HTTPClient.PostForm(apiURL, values)
	if err != nil {
		response.ErrorCodes = append(response.ErrorCodes, err.Error())
		return
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		response.ErrorCodes = append(response.ErrorCodes, err.Error())
		return
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		response.ErrorCodes = append(response.ErrorCodes, err.Error())
		return
	}

	return
}

func getFormValue(r *http.Request, key string) (string, error) {
	err := r.ParseMultipartForm(PostMaxMemory)
	if err != nil && err != http.ErrNotMultipart {
		return "", err
	}

	if form := r.Form; len(form) > 0 {
		return form.Get(key), nil
	}

	if form := r.PostForm; len(form) > 0 {
		return form.Get(key), nil
	}

	if m := r.MultipartForm; m != nil {
		if len(m.Value) > 0 {
			if values := m.Value[key]; len(values) > 0 {
				return values[0], nil
			}
		}
	}

	return "", nil
}

// Get returns the hcaptcha `Response` of the current "r" request and reports whether was found or not.
func Get(r *http.Request) (Response, bool) {
	v := r.Context().Value(ResponseContextKey)
	if v != nil {
		if response, ok := v.(Response); ok {
			return response, true
		}
	}

	return Response{}, false
}

// HTMLForm is the default HTML form for clients.
// It's totally optional, use your own code for the best possible result depending on your web application.
// See `ParseForm` and `RenderForm` for more.
var HTMLForm = `<form action="%s" method="POST">
	    <script src="https://hcaptcha.com/1/api.js"></script>
		<div class="h-captcha" data-sitekey="%s"></div>
    	<input type="submit" name="button" value="OK">
</form>`

// ParseForm parses the `HTMLForm` with the necessary parameters and returns
// its result for render.
func ParseForm(dataSiteKey, postActionRelativePath string) string {
	return fmt.Sprintf(HTMLForm, postActionRelativePath, dataSiteKey)
}

// RenderForm writes the `HTMLForm` to "w" response writer.
// See `_examples/basic/register_form.html` example for a custom form instead.
func RenderForm(w http.ResponseWriter, dataSiteKey, postActionRelativePath string) (int, error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return fmt.Fprint(w, ParseForm(dataSiteKey, postActionRelativePath))
}
