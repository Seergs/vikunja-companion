package relay

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/sideshow/apns2"
)

type stubAPNSClient struct {
	res *apns2.Response
	err error
	got *apns2.Notification
}

func (s *stubAPNSClient) PushWithContext(_ apns2.Context, n *apns2.Notification) (*apns2.Response, error) {
	s.got = n
	return s.res, s.err
}

func TestAPNSSendSuccess(t *testing.T) {
	stub := &stubAPNSClient{res: &apns2.Response{StatusCode: http.StatusOK}}
	a := &APNS{client: stub, topic: "com.example.app"}

	err := a.Send(context.Background(), "devtok", []byte(`{"aps":{}}`), "collapse-1")
	if err != nil {
		t.Fatal(err)
	}
	if stub.got.Topic != "com.example.app" || stub.got.DeviceToken != "devtok" || stub.got.CollapseID != "collapse-1" {
		t.Errorf("notification = %+v", stub.got)
	}
}

func TestAPNSSendBadTokenMapsToErrBadDeviceToken(t *testing.T) {
	for _, reason := range []string{apns2.ReasonBadDeviceToken, apns2.ReasonUnregistered, apns2.ReasonDeviceTokenNotForTopic} {
		stub := &stubAPNSClient{res: &apns2.Response{StatusCode: http.StatusGone, Reason: reason}}
		a := &APNS{client: stub, topic: "t"}
		err := a.Send(context.Background(), "d", []byte("{}"), "")
		if !errors.Is(err, ErrBadDeviceToken) {
			t.Errorf("reason %q -> err %v, want ErrBadDeviceToken", reason, err)
		}
	}
}

func TestAPNSSendOtherRejectionIsPlainError(t *testing.T) {
	stub := &stubAPNSClient{res: &apns2.Response{StatusCode: http.StatusInternalServerError, Reason: apns2.ReasonInternalServerError}}
	a := &APNS{client: stub, topic: "t"}
	err := a.Send(context.Background(), "d", []byte("{}"), "")
	if err == nil || errors.Is(err, ErrBadDeviceToken) {
		t.Errorf("err = %v, want a plain error", err)
	}
}

func TestAPNSSendTransportError(t *testing.T) {
	stub := &stubAPNSClient{err: errors.New("dial tcp: timeout")}
	a := &APNS{client: stub, topic: "t"}
	if err := a.Send(context.Background(), "d", []byte("{}"), ""); err == nil {
		t.Fatal("expected transport error")
	}
}
