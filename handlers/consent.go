package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	hydra "github.com/ory/hydra-client-go/v2"
	"github.com/traceshield/trace-shield/graph/common"
	"github.com/traceshield/trace-shield/graph/model"
	"github.com/traceshield/trace-shield/utils"
	"go.opentelemetry.io/otel/codes"
)

type ConsentAction string

const (
	ConsentActionAccept = ConsentAction("accept")
	ConsentActionDeny   = ConsentAction("deny")
)

type ConsentRequest struct {
	Challenge  string        `json:"consent_challenge"`
	Action     ConsentAction `json:"consent_action"`
	Remember   bool          `json:"remember"`
	GrantScope []string      `json:"grant_scope"`
}

func Consent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clients := common.GetContext(ctx)

	log := clients.Log.WithName("OAuthConsent")

	ctx, span := clients.Tracer.Start(ctx, "OAuthConsent")
	defer span.End()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error(err, "error during reading body")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	consentRequest := toConsentRequest(string(body))
	if consentRequest == nil {
		err := fmt.Errorf("error during parsing consent body")
		log.Error(err, "error during parsing consent request")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	consent, _, err := clients.HydraClient.OAuth2Api.GetOAuth2ConsentRequest(ctx).ConsentChallenge(consentRequest.Challenge).Execute()
	if err != nil {
		log.Error(err, "error during getting consent request")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userCtx := ForContext(ctx)

	accessToken := &model.ConsentRequestSessionAccessToken{}
	idToken := &model.ConsentRequestSessionIDToken{}

	if userCtx != nil {

		accessToken.Subject = &userCtx.Id
		idToken.Subject = &userCtx.Id

		if consentRequest.GrantScope != nil && &userCtx.Email != nil {
			if utils.StringContains(consentRequest.GrantScope, "email") {
				idToken.Email = &userCtx.Email
			}
		}
	}

	outSession := &hydra.AcceptOAuth2ConsentRequestSession{
		AccessToken: accessToken,
		IdToken:     idToken,
	}

	if consentRequest.Action == ConsentActionAccept {
		var rememberFor int64 = 3600
		acceptConsent, _, err := clients.HydraClient.OAuth2Api.AcceptOAuth2ConsentRequest(ctx).
			ConsentChallenge(consentRequest.Challenge).
			AcceptOAuth2ConsentRequest(hydra.AcceptOAuth2ConsentRequest{
				GrantAccessTokenAudience: consent.RequestedAccessTokenAudience,
				GrantScope:               consentRequest.GrantScope,
				Remember:                 &consentRequest.Remember,
				RememberFor:              &rememberFor,
				Session:                  outSession,
			}).
			Execute()
		if err != nil {
			log.Error(err, "error during accepting consent request")
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, acceptConsent.RedirectTo, http.StatusFound)
		return
	}

	rejectConsent, _, err := clients.HydraClient.OAuth2Api.RejectOAuth2ConsentRequest(ctx).
		ConsentChallenge(consentRequest.Challenge).
		RejectOAuth2Request(hydra.RejectOAuth2Request{
			Error:            nil,
			ErrorDebug:       nil,
			ErrorDescription: nil,
			ErrorHint:        nil,
			StatusCode:       nil,
		}).Execute()
	if err != nil {
		log.Error(err, "error during rejecting consent request")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, rejectConsent.RedirectTo, http.StatusFound)

}

func toConsentRequest(body string) *ConsentRequest {
	if len(body) == 0 {
		return nil
	}

	tuples := strings.Split(body, "&")
	if len(tuples) == 0 {
		return nil
	}

	result := new(ConsentRequest)
	for _, tupleString := range tuples {
		tuple := strings.Split(tupleString, "=")
		if len(tuple) != 2 {
			continue
		}

		key := tuple[0]
		value := tuple[1]

		switch key {
		case "consent_challenge":
			result.Challenge = value
		case "consent_action":
			result.Action = ConsentAction(value)
		case "remember":
			result.Remember = value == "1"
		case "grant_scope":
			result.GrantScope = append(result.GrantScope, value)
		}
	}

	return result
}
