#!/usr/bin/env python3
"""AWS SDK (boto3) smoke test against NovaNas s3gw.

Exercises: PUT / GET / LIST / MULTIPART-UPLOAD / DELETE. Exits non-zero on
failure so CI can gate merges.
"""

from __future__ import annotations

import io
import os
import sys
import time
import uuid

import boto3  # type: ignore
from botocore.client import Config  # type: ignore


def env(name: str, default: str | None = None) -> str:
    v = os.getenv(name, default)
    if v is None:
        print(f"missing env {name}", file=sys.stderr)
        sys.exit(2)
    return v


def main() -> int:
    endpoint = env("S3_ENDPOINT", "https://localhost:9000")
    access = env("S3_ACCESS_KEY", "novanas")
    secret = env("S3_SECRET_KEY", "novanas-secret")
    region = env("S3_REGION", "us-east-1")

    s3 = boto3.client(
        "s3",
        endpoint_url=endpoint,
        aws_access_key_id=access,
        aws_secret_access_key=secret,
        region_name=region,
        config=Config(signature_version="s3v4", s3={"addressing_style": "path"}),
        verify=False,
    )

    bucket = f"e2e-boto3-{uuid.uuid4().hex[:8]}"
    key = "hello.txt"
    payload = b"hello from boto3 smoke " + str(time.time()).encode()

    s3.create_bucket(Bucket=bucket)
    try:
        s3.put_object(Bucket=bucket, Key=key, Body=payload)
        got = s3.get_object(Bucket=bucket, Key=key)["Body"].read()
        assert got == payload, "GET payload mismatch"

        listed = {o["Key"] for o in s3.list_objects_v2(Bucket=bucket).get("Contents", [])}
        assert key in listed, "LIST missing key"

        # Multipart (10MiB, 2 parts of 5MiB each)
        mkey = "multipart.bin"
        mpu = s3.create_multipart_upload(Bucket=bucket, Key=mkey)
        parts = []
        for i in range(1, 3):
            body = io.BytesIO(b"A" * (5 * 1024 * 1024))
            r = s3.upload_part(
                Bucket=bucket, Key=mkey, UploadId=mpu["UploadId"],
                PartNumber=i, Body=body,
            )
            parts.append({"ETag": r["ETag"], "PartNumber": i})
        s3.complete_multipart_upload(
            Bucket=bucket, Key=mkey, UploadId=mpu["UploadId"],
            MultipartUpload={"Parts": parts},
        )
        head = s3.head_object(Bucket=bucket, Key=mkey)
        assert head["ContentLength"] == 10 * 1024 * 1024, "multipart size mismatch"
        print("boto3-smoke: PASS")
    finally:
        for obj in s3.list_objects_v2(Bucket=bucket).get("Contents", []):
            s3.delete_object(Bucket=bucket, Key=obj["Key"])
        s3.delete_bucket(Bucket=bucket)
    return 0


if __name__ == "__main__":
    sys.exit(main())
