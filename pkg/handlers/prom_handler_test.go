package handlers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestExtractNamespaceQuotaCapacitiesPreferLimits(t *testing.T) {
	quotas := []corev1.ResourceQuota{
		{
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceLimitsCPU:      resource.MustParse("4"),
					corev1.ResourceRequestsCPU:    resource.MustParse("6"),
					corev1.ResourceLimitsMemory:   resource.MustParse("8Gi"),
					corev1.ResourceRequestsMemory: resource.MustParse("12Gi"),
				},
			},
		},
	}

	cpu, memory, hasCPU, hasMemory := extractNamespaceQuotaCapacities(quotas)
	memoryLimit := resource.MustParse("8Gi")
	if !hasCPU || cpu != 4 {
		t.Fatalf("expected cpu quota 4 cores, got hasCPU=%v cpu=%v", hasCPU, cpu)
	}
	if !hasMemory || memory != float64(memoryLimit.Value()) {
		t.Fatalf("expected memory quota 8Gi, got hasMemory=%v memory=%v", hasMemory, memory)
	}
}

func TestExtractNamespaceQuotaCapacitiesFallbackToRequestsAndSpecHard(t *testing.T) {
	quotas := []corev1.ResourceQuota{
		{
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceRequestsCPU:    resource.MustParse("1500m"),
					corev1.ResourceRequestsMemory: resource.MustParse("2Gi"),
				},
			},
		},
	}

	cpu, memory, hasCPU, hasMemory := extractNamespaceQuotaCapacities(quotas)
	memoryRequest := resource.MustParse("2Gi")
	if !hasCPU || cpu != 1.5 {
		t.Fatalf("expected cpu quota 1.5 cores, got hasCPU=%v cpu=%v", hasCPU, cpu)
	}
	if !hasMemory || memory != float64(memoryRequest.Value()) {
		t.Fatalf("expected memory quota 2Gi, got hasMemory=%v memory=%v", hasMemory, memory)
	}
}

func TestExtractNamespaceQuotaCapacitiesNoQuota(t *testing.T) {
	quotas := []corev1.ResourceQuota{
		{
			Status: corev1.ResourceQuotaStatus{Hard: corev1.ResourceList{}},
		},
	}

	cpu, memory, hasCPU, hasMemory := extractNamespaceQuotaCapacities(quotas)
	if hasCPU || cpu != 0 {
		t.Fatalf("expected no cpu quota, got hasCPU=%v cpu=%v", hasCPU, cpu)
	}
	if hasMemory || memory != 0 {
		t.Fatalf("expected no memory quota, got hasMemory=%v memory=%v", hasMemory, memory)
	}
}
