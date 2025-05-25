package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

type EmailRequestToResend struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Html    string `json:"html"`
}

type IncomingPayload struct {
	Subject string `json:"subject"`
	Html    string `json:"html"`
}

func createResponseWithCORS(statusCode int, body string, requestOrigin string) (*events.APIGatewayProxyResponse, error) {
	headers := map[string]string{
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
		"Access-Control-Allow-Methods": "POST, OPTIONS",
	}

	netlifyContext := os.Getenv("CONTEXT")
	allowedProdOrigin := os.Getenv("ALLOWED_ORIGIN_PROD")

	if netlifyContext == "dev" {
		if requestOrigin != "" {
			headers["Access-Control-Allow-Origin"] = requestOrigin
		} else {
			headers["Access-Control-Allow-Origin"] = "*"
		}
	} else if requestOrigin == allowedProdOrigin && allowedProdOrigin != "" {
		headers["Access-Control-Allow-Origin"] = allowedProdOrigin
	}

	return &events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

func handler(request events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	requestOrigin := request.Headers["origin"]
	if requestOrigin == "" {
		requestOrigin = request.Headers["Origin"]
	}

	if request.HTTPMethod == "OPTIONS" {
		return createResponseWithCORS(http.StatusOK, "", requestOrigin)
	}

	var payload IncomingPayload
	err := json.Unmarshal([]byte(request.Body), &payload)
	if err != nil {
		return createResponseWithCORS(http.StatusBadRequest, "Invalid JSON payload: "+err.Error(), requestOrigin)
	}

	if payload.Subject == "" {
		return createResponseWithCORS(http.StatusBadRequest, "Missing 'subject' in request payload", requestOrigin)
	}
	if payload.Html == "" {
		return createResponseWithCORS(http.StatusBadRequest, "Missing 'html' in request payload", requestOrigin)
	}

	recipientEmail := os.Getenv("ALERT_EMAIL")
	senderEmail := os.Getenv("RESEND_FROM_EMAIL")
	resendAPIKey := os.Getenv("RESEND_API_KEY")

	if recipientEmail == "" || senderEmail == "" || resendAPIKey == "" {
		return createResponseWithCORS(http.StatusInternalServerError, "Missing required environment variables", requestOrigin)
	}

	emailForResend := EmailRequestToResend{
		To:      recipientEmail,
		From:    senderEmail,
		Subject: payload.Subject,
		Html:    payload.Html,
	}

	emailBody, err := json.Marshal(emailForResend)
	if err != nil {
		return createResponseWithCORS(http.StatusInternalServerError, "Error preparing email data", requestOrigin)
	}

	apiReq, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(emailBody))
	if err != nil {
		return createResponseWithCORS(http.StatusInternalServerError, "Error creating request to Resend", requestOrigin)
	}
	apiReq.Header.Set("Authorization", "Bearer "+resendAPIKey)
	apiReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	apiResp, err := client.Do(apiReq)
	if err != nil {
		return createResponseWithCORS(http.StatusInternalServerError, "Failed to send email (network error)", requestOrigin)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(apiResp.Body)

	if apiResp.StatusCode != http.StatusOK && apiResp.StatusCode != http.StatusCreated && apiResp.StatusCode != http.StatusAccepted {
		return createResponseWithCORS(http.StatusInternalServerError, "Resend API error (status: "+http.StatusText(apiResp.StatusCode)+")", requestOrigin)
	}

	return createResponseWithCORS(http.StatusOK, "Email processed successfully.", requestOrigin)
}

func main() {
	lambda.Start(handler)
}
