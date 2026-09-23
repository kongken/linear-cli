package linearconst

// API and upload endpoints for the Linear GraphQL API.

const (
	WebBaseURL         = "https://linear.app"
	APIEndpoint        = "https://api.linear.app/graphql"
	PrivateUploadHost  = "uploads.linear.app"
	PublicUploadHost   = "public.linear.app"
)

// UploadHostnames are Linear upload CDN hosts.
var UploadHostnames = []string{PrivateUploadHost, PublicUploadHost}
