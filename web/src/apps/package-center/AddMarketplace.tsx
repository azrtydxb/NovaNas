import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Icon } from "../../components/Icon";

type Props = { onClose: () => void };

export function AddMarketplace({ onClose }: Props) {
  const [name, setName] = useState("");
  const [indexUrl, setIndexUrl] = useState("");
  const [trustKeyUrl, setTrustKeyUrl] = useState("");
  const [trustKeyPem, setTrustKeyPem] = useState("");
  const qc = useQueryClient();

  const create = useMutation({
    mutationFn: () =>
      api(`/api/v1/marketplaces`, {
        method: "POST",
        body: JSON.stringify({
          name,
          indexUrl,
          trustKeyUrl: trustKeyUrl || undefined,
          trustKeyPem: trustKeyPem || undefined,
        }),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["marketplaces"] });
      onClose();
    },
  });

  const valid = name.trim() && indexUrl.trim() && (trustKeyUrl.trim() || trustKeyPem.trim());

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal__head">
          <div className="modal__icon">
            <Icon name="plus" size={16} />
          </div>
          <div className="modal__head-meta">
            <div className="modal__title">Add marketplace</div>
            <div className="muted modal__sub">
              Each marketplace ships with its own pinned cosign public key. The engine
              verifies every artifact in that index against this key.
            </div>
          </div>
          <button className="modal__close" onClick={onClose} aria-label="Close">
            <Icon name="x" size={14} />
          </button>
        </div>
        <div className="modal__body">
          <Field label="Name" hint="Stable identifier (e.g. truecharts).">
            <input
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="truecharts"
              autoFocus
            />
          </Field>
          <Field label="Index URL" hint="HTTPS URL serving the marketplace's index.json.">
            <input
              className="input"
              value={indexUrl}
              onChange={(e) => setIndexUrl(e.target.value)}
              placeholder="https://example.com/charts/index.json"
            />
          </Field>
          <Field
            label="Trust key URL"
            hint="Either provide a URL to fetch the cosign public key (PEM), or paste the PEM below."
          >
            <input
              className="input"
              value={trustKeyUrl}
              onChange={(e) => setTrustKeyUrl(e.target.value)}
              placeholder="https://example.com/charts/cosign.pub"
            />
          </Field>
          <Field label="Trust key PEM (optional)" hint="Use this if the key isn't reachable by URL.">
            <textarea
              className="input"
              rows={6}
              value={trustKeyPem}
              onChange={(e) => setTrustKeyPem(e.target.value)}
              placeholder="-----BEGIN PUBLIC KEY-----&#10;...&#10;-----END PUBLIC KEY-----"
              style={{ fontFamily: "var(--font-mono)", fontSize: 11, resize: "vertical" }}
            />
          </Field>
          {create.isError && (
            <div className="modal__err">Failed: {(create.error as Error).message}</div>
          )}
        </div>
        <div className="modal__foot">
          <button className="btn" onClick={onClose} disabled={create.isPending}>
            Cancel
          </button>
          <button
            className="btn btn--primary"
            disabled={!valid || create.isPending}
            onClick={() => create.mutate()}
          >
            <Icon name="plus" size={11} />
            {create.isPending ? "Adding…" : "Add marketplace"}
          </button>
        </div>
      </div>
    </div>
  );
}

function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="field">
      <label className="field__label">{label}</label>
      {children}
      {hint && <div className="field__hint muted">{hint}</div>}
    </div>
  );
}
