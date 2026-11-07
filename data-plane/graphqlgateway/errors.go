package graphqlgateway

import "fmt"

// serviceError carries a CrudService-shaped failure (status/code/message/fieldErrors) through
// graphql-go's error path as a GraphQL extensions object — the same extensions.code/status/
// fieldErrors shape every other transport in this migration uses, so a frontend's error
// handling doesn't need a gateway-specific branch.
type serviceError struct {
	status      uint16
	code        string
	message     string
	fieldErrors map[string][]string
}

func newServiceError(status uint16, code string, message *string, fieldErrors map[string][]string) *serviceError {
	msg := code
	if message != nil {
		msg = *message
	}
	return &serviceError{status: status, code: code, message: msg, fieldErrors: fieldErrors}
}

func (e *serviceError) Error() string {
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

// Extensions is picked up by graphql-go's gqlerrors formatting when an error returned from a
// resolver implements this interface.
func (e *serviceError) Extensions() map[string]interface{} {
	ext := map[string]interface{}{"code": e.code, "status": e.status}
	if e.fieldErrors != nil {
		ext["fieldErrors"] = e.fieldErrors
	}
	return ext
}
