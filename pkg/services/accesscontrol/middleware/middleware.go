package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/accesscontrol"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/util"
	"github.com/grafana/grafana/pkg/web"
)

func Middleware(ac accesscontrol.AccessControl) func(web.Handler, accesscontrol.Evaluator) web.Handler {
	return func(fallback web.Handler, evaluator accesscontrol.Evaluator) web.Handler {
		if ac.IsDisabled() {
			return fallback
		}

		return func(c *models.ReqContext) {
			injected, err := evaluator.Inject(buildScopeParams(c))
			if err != nil {
				c.JsonApiErr(http.StatusInternalServerError, "Internal server error", err)
				return
			}

			hasAccess, err := ac.Evaluate(c.Req.Context(), c.SignedInUser, injected)
			if !hasAccess || err != nil {
				Deny(c, injected, err)
				return
			}
		}
	}
}

func Deny(c *models.ReqContext, evaluator accesscontrol.Evaluator, err error) {
	id := newID()
	if err != nil {
		c.Logger.Error("Error from access control system", "error", err, "accessErrorID", id)
	} else {
		c.Logger.Info(
			"Access denied",
			"userID", c.UserId,
			"accessErrorID", id,
			"permissions", evaluator.String(),
		)
	}

	if !c.IsApiRequest() {
		// TODO(emil): I'd like to show a message after this redirect, not sure how that can be done?
		c.Redirect(setting.AppSubUrl + "/")
		return
	}

	// If the user triggers an error in the access control system, we
	// don't want the user to be aware of that, so the user gets the
	// same information from the system regardless of if it's an
	// internal server error or access denied.
	c.JSON(http.StatusForbidden, map[string]string{
		"title":         "Access denied", // the component needs to pick this up
		"message":       fmt.Sprintf("Your user account does not have permissions to do the action. We recorded your attempt with log message %s. Contact your administrator for help.", id),
		"accessErrorId": id,
	})
}

func newID() string {
	// Less ambiguity than alphanumerical.
	numerical := []byte("0123456789")
	id, err := util.GetRandomString(10, numerical...)
	if err != nil {
		// this should not happen, but if it does, a timestamp is as
		// useful as anything.
		id = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return "ACE" + id
}

func buildScopeParams(c *models.ReqContext) accesscontrol.ScopeParams {
	return accesscontrol.ScopeParams{
		OrgID:     c.OrgId,
		URLParams: web.Params(c.Req),
	}
}
