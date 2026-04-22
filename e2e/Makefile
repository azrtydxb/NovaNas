# NovaNas E2E Makefile
#
# Targets run the different E2E suites. They all assume a deployed NovaNas
# reachable at $(NOVANAS_BASE_URL) unless otherwise documented.

NOVANAS_BASE_URL ?= https://localhost:8443
S3_ENDPOINT     ?= https://localhost:9000
S3_ACCESS_KEY   ?= novanas
S3_SECRET_KEY   ?= novanas-secret

.PHONY: help install ui api-smoke qemu-boot qemu-upgrade s3-mint s3-ceph s3-sdks \
        nfs-compat smb-compat perf-fio perf-nfs perf-smb clean

help:
	@echo "NovaNas E2E — make targets:"
	@echo "  install        Install Playwright + browsers"
	@echo "  ui             Run Playwright UI+API E2E"
	@echo "  api-smoke      Run Go API smokes"
	@echo "  qemu-boot      Boot built ISO in QEMU and smoke-test"
	@echo "  qemu-upgrade   Install prior release then upgrade"
	@echo "  s3-mint        MinIO mint compat suite"
	@echo "  s3-ceph        Ceph s3-tests compat suite"
	@echo "  s3-sdks        AWS SDK smokes (boto3, js, go)"
	@echo "  nfs-compat     NFS v4 compliance"
	@echo "  smb-compat     smbtorture"
	@echo "  perf-fio       fio baseline"
	@echo "  perf-nfs       NFS throughput"
	@echo "  perf-smb       SMB throughput"

install:
	pnpm install
	pnpm exec playwright install --with-deps chromium

ui:
	NOVANAS_BASE_URL=$(NOVANAS_BASE_URL) pnpm exec playwright test

api-smoke:
	cd api && go test -v -timeout 10m ./...

qemu-boot:
	bash qemu/smoke-boot.sh

qemu-upgrade:
	bash qemu/upgrade-test.sh

s3-mint:
	bash compat/s3/minio-mint.sh $(S3_ENDPOINT) $(S3_ACCESS_KEY) $(S3_SECRET_KEY)

s3-ceph:
	bash compat/s3/ceph-s3-tests.sh $(S3_ENDPOINT) $(S3_ACCESS_KEY) $(S3_SECRET_KEY)

s3-sdks:
	S3_ENDPOINT=$(S3_ENDPOINT) S3_ACCESS_KEY=$(S3_ACCESS_KEY) S3_SECRET_KEY=$(S3_SECRET_KEY) \
		python3 compat/s3/aws-sdk/boto3-smoke.py
	S3_ENDPOINT=$(S3_ENDPOINT) S3_ACCESS_KEY=$(S3_ACCESS_KEY) S3_SECRET_KEY=$(S3_SECRET_KEY) \
		node compat/s3/aws-sdk/js-sdk-smoke.mjs
	cd compat/s3/aws-sdk && S3_ENDPOINT=$(S3_ENDPOINT) S3_ACCESS_KEY=$(S3_ACCESS_KEY) \
		S3_SECRET_KEY=$(S3_SECRET_KEY) go run go-sdk-smoke.go

nfs-compat:
	bash compat/nfs/nfsv4-compliance.sh

smb-compat:
	bash compat/smb/smbtorture.sh

perf-fio:
	bash qemu/performance/fio-baseline.sh

perf-nfs:
	bash qemu/performance/nfs-throughput.sh

perf-smb:
	bash qemu/performance/smb-throughput.sh

clean:
	rm -rf playwright-report test-results node_modules
	rm -rf qemu/artifacts
