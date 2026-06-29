package sign

// BelgianFederalTSAURL is the RFC 3161 endpoint for the Belgian Federal Government
// Time Stamping Authority. Tokens are signed under the Belgian Root CA6 chain
// (listed in the EU Trusted List).
//
// NOTE: This endpoint is HTTP (no TLS). Prefer configuring an HTTPS TSA for
// production use when available.
const BelgianFederalTSAURL = "http://tsa.belgium.be/connect"
