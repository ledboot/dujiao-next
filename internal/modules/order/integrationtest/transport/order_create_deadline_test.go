package orderhttp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderhttp "github.com/dujiao-next/internal/modules/order/transport/http"
	orderpresenter "github.com/dujiao-next/internal/modules/order/transport/presenter"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/gin-gonic/gin"
)

type deadlinePaymentCreator struct {
	deadline time.Time
	provider string
}

func (p deadlinePaymentCreator) CreatePayment(orderhttp.CreatePaymentInput) (*orderhttp.CreatePaymentResult, error) {
	return &orderhttp.CreatePaymentResult{Payment: &paymentdomain.Payment{ProviderType: p.provider, ExpiredAt: &p.deadline}}, nil
}

type deadlineOrderCreator struct{ order *orderdomain.Order }

func (p deadlineOrderCreator) CreateOrder(orderhttp.CreateOrderInput) (*orderdomain.Order, error) {
	return p.order, nil
}
func (p deadlineOrderCreator) CreateGuestOrder(orderhttp.CreateGuestOrderInput) (*orderdomain.Order, error) {
	return p.order, nil
}

func TestCreateAndPay502ReturnsShortenedOrderDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, provider := range []string{constants.PaymentProvider502, constants.PaymentProviderEpay} {
		for _, guest := range []bool{false, true} {
			name := provider + "_member"
			if guest {
				name = provider + "_guest"
			}
			t.Run(name, func(t *testing.T) {
				original := time.Now().Add(15 * time.Minute).UTC()
				shortened := original.Add(-10 * time.Minute)
				order := &orderdomain.Order{ID: 1, OrderNo: "DJ-DEADLINE", ExpiresAt: &original, Children: []orderdomain.Order{{ID: 2, ExpiresAt: &original}}}
				h := orderhttp.NewCreateHandler(deadlineOrderCreator{order}, nil, nil, deadlinePaymentCreator{deadline: shortened, provider: provider})
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Set("user_id", uint(1))
				c.Request = httptest.NewRequest(http.MethodPost, "/create", strings.NewReader(`{"channel_id":1,"items":[{"product_id":1,"quantity":1}],"email":"guest@example.test","order_password":"test-pass"}`))
				c.Request.Header.Set("Content-Type", "application/json")
				if guest {
					h.CreateGuestOrderAndPay(c)
				} else {
					h.CreateOrderAndPay(c)
				}
				var body struct {
					Data struct {
						Order     orderpresenter.OrderDetail `json:"order"`
						ExpiresAt time.Time                  `json:"expires_at"`
					} `json:"data"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				want := original
				if provider == constants.PaymentProvider502 {
					want = shortened
				}
				if body.Data.Order.ExpiresAt == nil || !body.Data.Order.ExpiresAt.Equal(want) || len(body.Data.Order.Children) != 1 || !body.Data.Order.Children[0].ExpiresAt.Equal(want) {
					t.Fatalf("inconsistent order deadlines: %s", recorder.Body.String())
				}
			})
		}
	}
}
