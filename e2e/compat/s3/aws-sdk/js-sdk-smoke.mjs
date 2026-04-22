#!/usr/bin/env node
// AWS SDK for JavaScript (v3) smoke test against NovaNas s3gw.
// Exits non-zero on failure.
//
// Run: node js-sdk-smoke.mjs
// Requires: npm i @aws-sdk/client-s3 @aws-sdk/lib-storage

import { randomBytes, randomUUID } from "node:crypto";
import {
  S3Client,
  CreateBucketCommand,
  PutObjectCommand,
  GetObjectCommand,
  ListObjectsV2Command,
  DeleteObjectCommand,
  DeleteBucketCommand,
  HeadObjectCommand,
} from "@aws-sdk/client-s3";
import { Upload } from "@aws-sdk/lib-storage";

const endpoint = process.env.S3_ENDPOINT ?? "https://localhost:9000";
const accessKeyId = process.env.S3_ACCESS_KEY ?? "novanas";
const secretAccessKey = process.env.S3_SECRET_KEY ?? "novanas-secret";
const region = process.env.S3_REGION ?? "us-east-1";

// Self-signed certs in dev. Production E2E uses a real CA.
process.env.NODE_TLS_REJECT_UNAUTHORIZED = "0";

const s3 = new S3Client({
  endpoint,
  region,
  credentials: { accessKeyId, secretAccessKey },
  forcePathStyle: true,
});

async function streamToBuffer(stream) {
  const chunks = [];
  for await (const chunk of stream) chunks.push(chunk);
  return Buffer.concat(chunks);
}

async function main() {
  const bucket = `e2e-jssdk-${randomUUID().slice(0, 8)}`;
  const key = "hello.txt";
  const payload = Buffer.from(`hello from js-sdk ${Date.now()}`);

  await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  try {
    await s3.send(new PutObjectCommand({ Bucket: bucket, Key: key, Body: payload }));
    const got = await s3.send(new GetObjectCommand({ Bucket: bucket, Key: key }));
    const gotBuf = await streamToBuffer(got.Body);
    if (!gotBuf.equals(payload)) throw new Error("GET payload mismatch");

    const listed = await s3.send(new ListObjectsV2Command({ Bucket: bucket }));
    if (!(listed.Contents ?? []).some((o) => o.Key === key)) {
      throw new Error("LIST missing key");
    }

    // Multipart (10 MiB) via lib-storage Upload helper.
    const mkey = "multipart.bin";
    const bigBody = randomBytes(10 * 1024 * 1024);
    const uploader = new Upload({
      client: s3,
      params: { Bucket: bucket, Key: mkey, Body: bigBody },
      partSize: 5 * 1024 * 1024,
      queueSize: 2,
    });
    await uploader.done();
    const head = await s3.send(new HeadObjectCommand({ Bucket: bucket, Key: mkey }));
    if (head.ContentLength !== bigBody.length) throw new Error("multipart size mismatch");

    console.log("js-sdk-smoke: PASS");
  } finally {
    const list = await s3.send(new ListObjectsV2Command({ Bucket: bucket }));
    for (const o of list.Contents ?? []) {
      await s3.send(new DeleteObjectCommand({ Bucket: bucket, Key: o.Key }));
    }
    await s3.send(new DeleteBucketCommand({ Bucket: bucket }));
  }
}

main().catch((err) => {
  console.error("js-sdk-smoke: FAIL", err);
  process.exit(1);
});
