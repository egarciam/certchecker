package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	monitoringv1alpha1 "egarciam.com/checkcert/api/v1alpha1"
)

func TestGetCertificateStatus(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expiry    time.Time
		threshold int
		want      string
	}{
		{name: "expired", expiry: now.Add(-time.Hour), threshold: 30, want: expired},
		{name: "expiring", expiry: now.Add(29 * 24 * time.Hour), threshold: 30, want: expiring},
		{name: "exactly at the threshold is still valid", expiry: now.Add(30 * 24 * time.Hour), threshold: 30, want: valid},
		{name: "valid", expiry: now.Add(31 * 24 * time.Hour), threshold: 30, want: valid},
		{name: "custom threshold classifies as expiring", expiry: now.Add(60 * 24 * time.Hour), threshold: 90, want: expiring},
		{name: "zero threshold only reports expired", expiry: now.Add(60 * 24 * time.Hour), threshold: 0, want: valid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getCertificateStatus(tt.expiry, now, tt.threshold); got != tt.want {
				t.Fatalf("getCertificateStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmailShouldBeSent(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	lastSent := now.Add(-time.Hour).Format(time.RFC3339)

	tests := []struct {
		name     string
		status   monitoringv1alpha1.MonitoredCertificateStatus
		cooldown int
		want     bool
	}{
		{
			name:   "never sent",
			status: monitoringv1alpha1.MonitoredCertificateStatus{},
			want:   true,
		},
		{
			name:   "sent but timestamp missing",
			status: monitoringv1alpha1.MonitoredCertificateStatus{EmailSent: true},
			want:   true,
		},
		{
			name:   "sent but timestamp invalid",
			status: monitoringv1alpha1.MonitoredCertificateStatus{EmailSent: true, LastEmailSentAt: "not-a-date"},
			want:   true,
		},
		{
			name:     "cooldown still active",
			status:   monitoringv1alpha1.MonitoredCertificateStatus{EmailSent: true, LastEmailSentAt: lastSent},
			cooldown: 86400,
			want:     false,
		},
		{
			name:     "cooldown elapsed",
			status:   monitoringv1alpha1.MonitoredCertificateStatus{EmailSent: true, LastEmailSentAt: now.Add(-25 * time.Hour).Format(time.RFC3339)},
			cooldown: 86400,
			want:     true,
		},
		{
			name:     "zero cooldown always sends",
			status:   monitoringv1alpha1.MonitoredCertificateStatus{EmailSent: true, LastEmailSentAt: lastSent},
			cooldown: 0,
			want:     true,
		},
		{
			name:     "exactly at the cooldown boundary sends",
			status:   monitoringv1alpha1.MonitoredCertificateStatus{EmailSent: true, LastEmailSentAt: now.Add(-time.Hour).Format(time.RFC3339)},
			cooldown: 3600,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := emailShouldBeSent(tt.status, tt.cooldown, now); got != tt.want {
				t.Fatalf("emailShouldBeSent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindMonitoredCert(t *testing.T) {
	statuses := []monitoringv1alpha1.MonitoredCertificateStatus{
		{Name: "internal-default.first", Status: expiring},
		{Name: "internal-default.second", Status: expired},
	}

	got := findMonitoredCert(statuses, "internal-default.second")
	if got.Status != expired {
		t.Fatalf("findMonitoredCert() status = %q, want %q", got.Status, expired)
	}

	missing := findMonitoredCert(statuses, "internal-default.missing")
	if missing.Name != "" {
		t.Fatalf("findMonitoredCert() for missing entry = %+v, want zero value", missing)
	}
}

func TestDiscoverInternalCertsDoesNotMarkFailedSendAsSent(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	secret := newTLSTestSecret(t, "default", "tls-expiring", now.Add(10*24*time.Hour))
	configMap := newRecipientsConfigMap()
	reconciler := newTestReconciler(t, now, []client.Object{secret, configMap})
	sendAttempts := 0
	reconciler.MailSender = func(string, string, string) error {
		sendAttempts++
		return errors.New("smtp unavailable")
	}

	monitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			DiscoverInternal:    true,
			SendMail:            true,
			EmailCooldown:       86400,
			ExpiryThresholdDays: 30,
		},
	}

	statuses, _, _, err := reconciler.discoverInternalCerts(context.Background(), monitor)
	if err != nil {
		t.Fatalf("discoverInternalCerts() error = %v", err)
	}
	if sendAttempts != 1 {
		t.Fatalf("send attempts = %d, want 1", sendAttempts)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if statuses[0].EmailSent {
		t.Fatal("failed send was marked as sent")
	}
	if statuses[0].LastEmailSentAt != "" {
		t.Fatalf("LastEmailSentAt = %q, want empty", statuses[0].LastEmailSentAt)
	}
	if statuses[0].Status != expiring {
		t.Fatalf("status = %q, want %q", statuses[0].Status, expiring)
	}
}

func TestDiscoverInternalCertsMarksSuccessfulSend(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	secret := newTLSTestSecret(t, "default", "tls-expiring", now.Add(10*24*time.Hour))
	reconciler := newTestReconciler(t, now, []client.Object{secret, newRecipientsConfigMap()})
	reconciler.MailSender = func(string, string, string) error { return nil }

	monitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			DiscoverInternal:    true,
			SendMail:            true,
			EmailCooldown:       86400,
			ExpiryThresholdDays: 30,
		},
	}

	statuses, summary, mailReady, err := reconciler.discoverInternalCerts(context.Background(), monitor)
	if err != nil {
		t.Fatalf("discoverInternalCerts() error = %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if !statuses[0].EmailSent {
		t.Fatal("successful send was not marked as sent")
	}
	if statuses[0].LastEmailSentAt != now.Format(time.RFC3339) {
		t.Fatalf("LastEmailSentAt = %q, want %q", statuses[0].LastEmailSentAt, now.Format(time.RFC3339))
	}
	if summary.Total != 1 || summary.Expiring != 1 || summary.Valid != 0 || summary.Expired != 0 {
		t.Fatalf("summary = %+v, want total=1 expiring=1", summary)
	}
	if !mailReady {
		t.Fatal("mailReady = false, want true")
	}
}

func TestDiscoverInternalCertsRefreshesStatusDuringCooldown(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	secret := newTLSTestSecret(t, "default", "tls-now-expired", now.Add(-time.Hour))
	reconciler := newTestReconciler(t, now, []client.Object{secret, newRecipientsConfigMap()})
	sendAttempts := 0
	reconciler.MailSender = func(string, string, string) error {
		sendAttempts++
		return nil
	}

	previousLastSent := now.Add(-time.Hour)
	monitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			DiscoverInternal:    true,
			SendMail:            true,
			EmailCooldown:       86400,
			ExpiryThresholdDays: 30,
		},
		Status: monitoringv1alpha1.CertificateMonitorStatus{
			MonitoredCertificates: []monitoringv1alpha1.MonitoredCertificateStatus{
				{
					Name:            "internal-default.tls-now-expired",
					Status:          expiring,
					Expiry:          now.Add(24 * time.Hour).Format(time.RFC3339),
					Namespace:       "default",
					EmailSent:       true,
					LastEmailSentAt: previousLastSent.Format(time.RFC3339),
				},
			},
		},
	}

	statuses, _, _, err := reconciler.discoverInternalCerts(context.Background(), monitor)
	if err != nil {
		t.Fatalf("discoverInternalCerts() error = %v", err)
	}
	if sendAttempts != 0 {
		t.Fatalf("send attempts = %d, want 0 during cooldown", sendAttempts)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	if statuses[0].Status != expired {
		t.Fatalf("status = %q, want fresh status %q", statuses[0].Status, expired)
	}
	if statuses[0].Expiry != now.Add(-time.Hour).Format(time.RFC3339) {
		t.Fatalf("Expiry = %q, want refreshed expiry", statuses[0].Expiry)
	}
	if !statuses[0].EmailSent {
		t.Fatal("previous email metadata was not preserved during cooldown")
	}
	if statuses[0].LastEmailSentAt != previousLastSent.Format(time.RFC3339) {
		t.Fatalf("LastEmailSentAt = %q, want %q", statuses[0].LastEmailSentAt, previousLastSent.Format(time.RFC3339))
	}
}

func TestDiscoverInternalCertsContinuesWhenRecipientsAreMissing(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	secret := newTLSTestSecret(t, "default", "tls-expired", now.Add(-time.Hour))
	reconciler := newTestReconciler(t, now, []client.Object{secret})
	sendAttempts := 0
	reconciler.MailSender = func(string, string, string) error {
		sendAttempts++
		return nil
	}

	monitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			DiscoverInternal:    true,
			SendMail:            true,
			EmailCooldown:       86400,
			ExpiryThresholdDays: 30,
		},
	}

	statuses, _, _, err := reconciler.discoverInternalCerts(context.Background(), monitor)
	if err != nil {
		t.Fatalf("discoverInternalCerts() error = %v", err)
	}
	if sendAttempts != 0 {
		t.Fatalf("send attempts = %d, want 0 without recipients", sendAttempts)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1 despite missing recipients", len(statuses))
	}
	if statuses[0].EmailSent {
		t.Fatal("email marked as sent without recipients")
	}
}

func TestDiscoverInternalCertsCountsInvalidCertificates(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	invalidSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-invalid", Namespace: "default"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{"tls.crt": []byte("not a PEM certificate")},
	}
	reconciler := newTestReconciler(t, now, []client.Object{invalidSecret})
	monitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			DiscoverInternal:    true,
			ExpiryThresholdDays: 30,
		},
	}

	statuses, summary, _, err := reconciler.discoverInternalCerts(context.Background(), monitor)
	if err != nil {
		t.Fatalf("discoverInternalCerts() error = %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("statuses = %d, want 0", len(statuses))
	}
	if summary.Errors != 1 {
		t.Fatalf("summary.Errors = %d, want 1", summary.Errors)
	}
	if summary.Total != 0 {
		t.Fatalf("summary.Total = %d, want 0", summary.Total)
	}
}

func TestMapSecretToMonitors(t *testing.T) {
	monitor1 := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: "first", Namespace: "team-a"},
	}
	monitor2 := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: "second", Namespace: "team-b"},
	}
	reconciler := newTestReconciler(t, time.Now(), []client.Object{monitor1, monitor2})

	requests := reconciler.mapSecretToMonitors(context.Background(), &corev1.Secret{
		Type: corev1.SecretTypeTLS,
	})
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
}

func TestMapRecipientsConfigMapToMonitors(t *testing.T) {
	defaultConfigMonitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: "default-config", Namespace: "team-a"},
	}
	customConfigMonitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-config", Namespace: "team-b"},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			EmailRecipientsConfigMap: "custom-recipients",
			EmailRecipientsNamespace: "ops",
		},
	}
	reconciler := newTestReconciler(t, time.Now(), []client.Object{defaultConfigMonitor, customConfigMonitor})

	defaultConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "email-recipients-config", Namespace: "team-a"},
		Data:       map[string]string{"emails": `["team-a@example.com"]`},
	}
	requests := reconciler.mapRecipientsConfigMapToMonitors(context.Background(), defaultConfigMap)
	if len(requests) != 1 || requests[0].Namespace != "team-a" || requests[0].Name != "default-config" {
		t.Fatalf("default config requests = %+v, want only team-a/default-config", requests)
	}

	customConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-recipients", Namespace: "ops"},
		Data:       map[string]string{"emails": `["ops@example.com"]`},
	}
	requests = reconciler.mapRecipientsConfigMapToMonitors(context.Background(), customConfigMap)
	if len(requests) != 1 || requests[0].Namespace != "team-b" || requests[0].Name != "custom-config" {
		t.Fatalf("custom config requests = %+v, want only team-b/custom-config", requests)
	}
}

func TestGetRecipientsUsesConfiguredConfigMap(t *testing.T) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-recipients", Namespace: "ops"},
		Data:       map[string]string{"emails": `["ops@example.com"]`},
	}
	reconciler := newTestReconciler(t, time.Now(), []client.Object{configMap})
	monitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			EmailRecipientsConfigMap: "custom-recipients",
			EmailRecipientsNamespace: "ops",
		},
	}

	recipients, err := reconciler.getRecipients(context.Background(), monitor)
	if err != nil {
		t.Fatalf("getRecipients() error = %v", err)
	}
	if len(recipients) != 1 || recipients[0] != "ops@example.com" {
		t.Fatalf("recipients = %v, want [ops@example.com]", recipients)
	}
}

func TestReconcileUpdatesStatusAndConditions(t *testing.T) {
	now := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	secret := newTLSTestSecret(t, "default", "tls-expiring", now.Add(10*24*time.Hour))
	monitor := &monitoringv1alpha1.CertificateMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-monitor",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: monitoringv1alpha1.CertificateMonitorSpec{
			DiscoverInternal:    true,
			SendMail:            false,
			CheckInterval:       60,
			ExpiryThresholdDays: 30,
		},
	}
	reconciler := newTestReconciler(t, now, []client.Object{secret, monitor})
	request := reconcile.Request{NamespacedName: types.NamespacedName{Name: monitor.Name, Namespace: monitor.Namespace}}
	if err := reconciler.Get(context.Background(), request.NamespacedName, &monitoringv1alpha1.CertificateMonitor{}); err != nil {
		t.Fatalf("precondition: monitor not found: %v", err)
	}

	result, err := reconciler.Reconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Fatalf("RequeueAfter = %v, want 1m", result.RequeueAfter)
	}

	updated := &monitoringv1alpha1.CertificateMonitor{}
	if err := reconciler.Get(context.Background(), request.NamespacedName, updated); err != nil {
		t.Fatalf("get updated monitor: %v", err)
	}
	if updated.Status.ObservedGeneration != 1 {
		t.Fatalf("ObservedGeneration = %d, want 1", updated.Status.ObservedGeneration)
	}
	if updated.Status.LastChecked != now.Format(time.RFC3339) {
		t.Fatalf("LastChecked = %q, want %q", updated.Status.LastChecked, now.Format(time.RFC3339))
	}
	if updated.Status.Summary.Total != 1 || updated.Status.Summary.Expiring != 1 {
		t.Fatalf("Summary = %+v, want total=1 expiring=1", updated.Status.Summary)
	}
	if len(updated.Status.MonitoredCertificates) != 1 {
		t.Fatalf("MonitoredCertificates = %d, want 1", len(updated.Status.MonitoredCertificates))
	}
	ready := meta.FindStatusCondition(updated.Status.Conditions, "Ready")
	if ready == nil || ready.Status != metav1.ConditionTrue {
		t.Fatalf("Ready condition = %+v, want True", ready)
	}
	degraded := meta.FindStatusCondition(updated.Status.Conditions, "Degraded")
	if degraded == nil || degraded.Status != metav1.ConditionFalse {
		t.Fatalf("Degraded condition = %+v, want False", degraded)
	}
}

func newTestReconciler(t *testing.T, now time.Time, objects []client.Object) *CertificateMonitorReconciler {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := monitoringv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add monitoring scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&monitoringv1alpha1.CertificateMonitor{}).
		WithIndex(&corev1.Secret{}, "type", func(obj client.Object) []string {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				return nil
			}
			return []string{string(secret.Type)}
		}).
		WithObjects(objects...).
		Build()

	return &CertificateMonitorReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Now:    func() time.Time { return now },
	}
}

func newTLSTestSecret(t *testing.T, namespace, name string, notAfter time.Time) *corev1.Secret {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    notAfter.Add(-24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Type:       corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		},
	}
}

func newRecipientsConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "email-recipients-config", Namespace: "default"},
		Data:       map[string]string{"emails": `["alerts@example.com"]`},
	}
}
