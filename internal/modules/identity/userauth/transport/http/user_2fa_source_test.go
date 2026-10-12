package userauthhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	captcha "github.com/dujiao-next/internal/modules/captcha/contract"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"

	"github.com/gin-gonic/gin"
)

type sourceTestTOTPService struct{}

func (sourceTestTOTPService) GetStatus(uint) (*UserTOTPStatus, error) {
	return &UserTOTPStatus{}, nil
}
func (sourceTestTOTPService) Setup(uint) (*UserTOTPSetupResult, error) {
	return &UserTOTPSetupResult{}, nil
}
func (sourceTestTOTPService) Enable(uint, string) (*UserTOTPEnableResult, error) {
	return &UserTOTPEnableResult{}, nil
}
func (sourceTestTOTPService) Disable(uint, string, bool) error { return nil }
func (sourceTestTOTPService) RegenerateRecoveryCodes(uint, string) ([]string, error) {
	return nil, nil
}
func (sourceTestTOTPService) VerifyChallengeCode(uint, string) error         { return nil }
func (sourceTestTOTPService) VerifyChallengeRecoveryCode(uint, string) error { return nil }

type sourceTest2FAAuth struct {
	source string
}

func (s sourceTest2FAAuth) ParseUserChallengeToken(string) (*UserChallengeClaims, error) {
	return &UserChallengeClaims{
		UserID:      42,
		JTI:         "source-test-jti",
		LoginSource: s.source,
	}, nil
}

func (sourceTest2FAAuth) CompleteLoginAfter2FA(context.Context, uint, bool) (*AuthLoginResult, error) {
	now := time.Now()
	return &AuthLoginResult{
		User: &userdomain.User{
			ID:    42,
			Email: "buyer@example.com",
		},
		Token:     "access-token",
		ExpiresAt: now.Add(time.Hour),
	}, nil
}

func (sourceTest2FAAuth) GetUserEmail(uint) (string, error) {
	return "buyer@example.com", nil
}

type sourceTestChallengeStore struct{}

func (sourceTestChallengeStore) IsRevoked(context.Context, string) bool { return false }
func (sourceTestChallengeStore) BumpFails(context.Context, string) int64 {
	return 0
}
func (sourceTestChallengeStore) Revoke(context.Context, string) {}

type sourceTestLoginRecord struct {
	source string
	status string
}

type sourceTestLoginRecorder struct {
	records []sourceTestLoginRecord
}

func (r *sourceTestLoginRecorder) Record(_ string, _ uint, status, _, source, _, _, _ string) {
	r.records = append(r.records, sourceTestLoginRecord{source: source, status: status})
}

func TestVerifyUser2FAPreservesExternalLoginSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, source := range []string{constants.LoginLogSourceGoogle, constants.LoginLogSourceTelegram} {
		t.Run(source, func(t *testing.T) {
			loginRecorder := &sourceTestLoginRecorder{}
			handler := NewUser2FAHandler(
				sourceTestTOTPService{},
				sourceTest2FAAuth{source: source},
				sourceTestChallengeStore{},
				loginRecorder,
			)
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/v1/auth/login/verify-2fa",
				bytes.NewBufferString(`{"challenge_token":"challenge","code":"123456"}`),
			)
			context.Request.Header.Set("Content-Type", "application/json")

			handler.VerifyUser2FA(context)

			if len(loginRecorder.records) != 1 {
				t.Fatalf("login records = %d, want 1", len(loginRecorder.records))
			}
			if loginRecorder.records[0].source != source {
				t.Fatalf("login source = %q, want %q", loginRecorder.records[0].source, source)
			}
			if loginRecorder.records[0].status != constants.LoginLogStatusSuccess {
				t.Fatalf("login status = %q", loginRecorder.records[0].status)
			}
		})
	}
}

type captchaLoginSettings struct{ verifyEmail bool }

func (captchaLoginSettings) GetRegistrationEnabled(bool) (bool, error) { return true, nil }
func (s captchaLoginSettings) GetEmailVerificationEnabled(bool) (bool, error) {
	return s.verifyEmail, nil
}

type captchaLoginAuth struct{ calls int }

func (a *captchaLoginAuth) Register(context.Context, string, string, string, string, bool, bool) (*userdomain.User, string, time.Time, error) {
	a.calls++
	return &userdomain.User{ID: 1, Email: "buyer@example.com"}, "token", time.Now().Add(time.Hour), nil
}
func (a *captchaLoginAuth) LoginStep1(context.Context, string, string, bool) (*AuthLoginResult, error) {
	a.calls++
	return &AuthLoginResult{User: &userdomain.User{ID: 1, Email: "buyer@example.com"}, Token: "token", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

type captchaLoginVerifier struct {
	err          error
	scene, token string
}

func (v *captchaLoginVerifier) Verify(scene string, payload captchahttp.CaptchaPayloadRequest, _ string) error {
	v.scene, v.token = scene, payload.TurnstileToken
	return v.err
}

func TestPasswordAuthRequiresCaptchaBeforeBusinessLogic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"login", "register"} {
		for _, verifyEmail := range []bool{true, false} {
			for _, tc := range []struct {
				name, token   string
				err           error
				status, calls int
			}{
				{"missing", "", captcha.ErrRequired, http.StatusBadRequest, 0},
				{"invalid", "bad-token", captcha.ErrInvalid, http.StatusBadRequest, 0},
				{"configuration", "token", captcha.ErrConfigInvalid, http.StatusInternalServerError, 0},
				{"unavailable", "token", errors.New("upstream unavailable"), http.StatusInternalServerError, 0},
				{"verified", "valid-token", nil, 0, 1},
			} {
				t.Run(endpoint+"/"+tc.name+"/email="+map[bool]string{true: "on", false: "off"}[verifyEmail], func(t *testing.T) {
					auth := &captchaLoginAuth{}
					verifier := &captchaLoginVerifier{err: tc.err}
					handler := NewUserLoginHandler(captchaLoginSettings{verifyEmail}, auth, verifier, nil)
					router := gin.New()
					scene := constants.CaptchaSceneLogin
					if endpoint == "register" {
						router.POST("/auth/register", handler.UserRegister)
						scene = constants.CaptchaSceneRegisterSendCode
					} else {
						router.POST("/auth/login", handler.UserLogin)
					}
					body := `{"email":"buyer@example.com","password":"password123","agreement_accepted":true,"captcha_payload":{"turnstile_token":"` + tc.token + `"}}`
					request := httptest.NewRequest(http.MethodPost, "/auth/"+endpoint, strings.NewReader(body))
					request.Header.Set("Content-Type", "application/json")
					recorder := httptest.NewRecorder()
					router.ServeHTTP(recorder, request)
					var result struct {
						StatusCode int `json:"status_code"`
					}
					if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if result.StatusCode != tc.status || auth.calls != tc.calls {
						t.Fatalf("status=%d business calls=%d body=%s", result.StatusCode, auth.calls, recorder.Body.String())
					}
					if verifier.scene != scene || verifier.token != tc.token {
						t.Fatalf("wrong verification input: %+v", verifier)
					}
				})
			}
		}
	}
}
