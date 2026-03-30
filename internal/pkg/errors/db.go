// internal/pkg/errors/db.go
package apperr

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func FromDB(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return New(CodeNotFound, "resource not found", err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			switch pgErr.ConstraintName {
			case "sales_sale_number_key":
				return Wrap(CodeConflict,
					"This sale appears to have already been recorded. Please refresh and check your sales list.",
					err,
				)
			case "sales_idempotency_key_key":
				return Wrap(CodeConflict,
					"This request has already been processed.",
					err,
				)
			default:
				return Wrap(CodeConflict, "resource already exists", err)
			}

		case "23503": // foreign_key_violation
			return Wrap(CodeBadRequest, "one or more selected records are invalid", err)

		case "23514": // check_violation
			return Wrap(CodeBadRequest, "one or more submitted values are invalid", err)

		case "22P02": // invalid_text_representation
			return Wrap(CodeBadRequest, "invalid input format", err)

		default:
			return Wrap(CodeInternal, ErrInternal.Message, err)
		}
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate key value") {
		return Wrap(CodeConflict, "resource already exists", err)
	}

	return Wrap(CodeInternal, ErrInternal.Message, err)
}
