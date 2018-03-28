package urlshortener

import "errors"

var (
	errURLNotFound  = errors.New("This URL does not exist yet")
	errMalformedURL = errors.New("This URL is not valid")
)
