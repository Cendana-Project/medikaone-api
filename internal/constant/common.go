package constant

type ContextKey string

const (
	ProductionEnvironment = "production"

	RequestID = ContextKey("reqId")
	UserID    = ContextKey("user_id")
	TokenID   = ContextKey("jti")
	// ClientFingerprint is a keyed, truncated pseudonym of the caller IP set by
	// the trusted public-auth rate-limit middleware. Services use it to scope
	// identity counters without retaining a raw address.
	ClientFingerprint = ContextKey("client_fingerprint")
)
