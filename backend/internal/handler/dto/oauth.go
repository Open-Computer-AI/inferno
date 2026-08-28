package dto

// SelfHostedClientRequest is the body of POST /api/oauth/self-hosted-client.
type SelfHostedClientRequest struct {
	Name           string `json:"name"`
	RedirectOrigin string `json:"redirect_origin" binding:"required"`
}

// SelfHostedClientResponse is returned to `oc dashboard register`.
type SelfHostedClientResponse struct {
	ClientID string `json:"client_id"`
	Name     string `json:"name"`
}
