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

func handler(request events.APIGatewayProxyRequest) (*events.APIGatewayProxyResponse, error) {
	var payload IncomingPayload
	err := json.Unmarshal([]byte(request.Body), &payload)
	if err != nil {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Invalid JSON payload: " + err.Error(),
		}, nil
	}

	if payload.Subject == "" {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Missing 'subject' in request payload",
		}, nil
	}
	if payload.Html == "" {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Missing 'html' in request payload",
		}, nil
	}

	recipientEmail := os.Getenv("ALERT_EMAIL")
	senderEmail := os.Getenv("RESEND_FROM_EMAIL")
	resendAPIKey := os.Getenv("RESEND_API_KEY")

	if recipientEmail == "" || senderEmail == "" || resendAPIKey == "" {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Missing required environment variables (ALERT_EMAIL, RESEND_FROM_EMAIL, RESEND_API_KEY)",
		}, nil
	}

	emailForResend := EmailRequestToResend{
		To:      recipientEmail,
		From:    senderEmail,
		Subject: payload.Subject,
		Html:    payload.Html,
	}

	emailBody, err := json.Marshal(emailForResend)
	if err != nil {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Error preparing email data for Resend",
		}, nil
	}

	apiReq, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(emailBody))
	if err != nil {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Error creating request to Resend API",
		}, nil
	}
	apiReq.Header.Set("Authorization", "Bearer "+resendAPIKey)
	apiReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	apiResp, err := client.Do(apiReq)
	if err != nil {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Failed to send email via Resend (network error)",
		}, nil
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(apiResp.Body)

	if apiResp.StatusCode != http.StatusOK && apiResp.StatusCode != http.StatusCreated && apiResp.StatusCode != http.StatusAccepted {
		return &events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "Resend API returned an error (status: " + http.StatusText(apiResp.StatusCode) + ")",
		}, nil
	}

	return &events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       "Email processed successfully by Netlify Function.",
	}, nil
}

func main() {
	lambda.Start(handler)
}
