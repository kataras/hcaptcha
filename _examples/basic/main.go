package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"

	"github.com/kataras/hcaptcha"
)

// Get the following values from: https://dashboard.hcaptcha.com
// Also, check: https://docs.hcaptcha.com/#localdev to test on local environment.
var (
	siteKey   = os.Getenv("HCAPTCHA-SITE-KEY")
	secretKey = os.Getenv("HCAPTCHA-SECRET-KEY")
)

var (
	client       = hcaptcha.New(secretKey) /* See `Client.FailureHandler` too. */
	registerForm = template.Must(template.ParseFiles("./register_form.html"))
)

func main() {
	http.HandleFunc("/", renderForm)
	http.HandleFunc("/page", client.HandlerFunc(page) /* See `Client.SiteVerify` to get rid of a wrapper if necessary */)

	fmt.Printf("SiteKey = %s\tSecretKey = %s\nListening on: http://yourdomain.com\n",
		siteKey, secretKey)

	http.ListenAndServe(":80", nil)
}

func page(w http.ResponseWriter, r *http.Request) {
	hcaptchaResp, ok := hcaptcha.Get(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Are you a bot?")
		return
	}

	fmt.Fprintf(w, "Page is inspected by a Human.\nResponse value is: %#+v", hcaptchaResp)
}

func renderForm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	registerForm.Execute(w, map[string]string{
		"SiteKey": siteKey,
	})
}
