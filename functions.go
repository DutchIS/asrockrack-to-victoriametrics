package main

import (
	"bytes"
	"log"
	"encoding/json"
	"mime/multipart"
	"encoding/base64"
	"net/http"
	"net/http/cookiejar"
	"net/url"
    "crypto/tls"
)

func CreateSession(server Server) (cookie string, csrfToken string, err error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	err = writer.WriteField("username", server.Username)
	if err != nil {
		log.Fatalf("Failed to write form field: %v", err)
	}

	err = writer.WriteField("password", server.Password)
	if err != nil {
		log.Fatalf("Failed to write form field: %v", err)
	}

	err = writer.WriteField("certlogin", "0")
	if err != nil {
		log.Fatalf("Failed to write form field: %v", err)
	}

	err = writer.Close()
	if err != nil {
		log.Fatalf("Failed to close form: %v", err)
	}
	
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Failed to create cookie jar: %v", err)
	}

	httpClient := &http.Client{
		Jar: jar,
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	req, err := http.NewRequest("POST", "https://" + server.Hostname + "/api/session", body)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("Request failed with status code %d", resp.StatusCode)
	}

	// Decode the response body as JSON
	var respData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&respData)
	if err != nil {
		log.Printf("Failed to decode JSON response: %v", err)
		return "", "", err
	}
	
	log.Printf("Logged in successfully to %s", server.Hostname)

	u, _ := url.Parse("https://" + server.Hostname)
	cookies := jar.Cookies(u)
	for _, cookie := range cookies {
		if cookie.Name == "QSESSIONID" {
			return cookie.Value, respData["CSRFToken"].(string), nil
		}
	}

	return "", "", nil
}

func BasicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
