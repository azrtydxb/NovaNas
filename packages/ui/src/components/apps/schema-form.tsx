/**
 * JSON-Schema-driven form for App install values (issue #3).
 *
 * Thin wrapper around `@rjsf/core` + `@rjsf/validator-ajv8`, with minimal
 * Shadcn-flavored styling so the form visually fits the rest of the console.
 *
 * Apps expose their values schema at `App.spec.schema` (unknown in the Zod
 * type, but concretely a JSON Schema object). We pass it straight through to
 * RJSF. Defaults from `schema.default` (and nested `properties[*].default`)
 * are surfaced via RJSF's built-in `schema.default` handling; we also pre-seed
 * `formData` on mount from any top-level `schema.default` so the parent form
 * can reset cleanly.
 */
import { Button } from '@/components/ui/button';
import Form, { type IChangeEvent } from '@rjsf/core';
import type { UiSchema } from '@rjsf/utils';
import validator from '@rjsf/validator-ajv8';
import type React from 'react';
import { useMemo } from 'react';

// RJSF's generics propagate down into every template. To keep the wrapper
// simple we erase them by defaulting to `any` on both the schema and the
// validator side — the external API surface is still typed via FormData.
const RjsfForm = Form as unknown as React.ComponentType<{
  schema: unknown;
  uiSchema?: UiSchema;
  // biome-ignore lint/suspicious/noExplicitAny: RJSF validator generics.
  validator: any;
  formData?: Record<string, unknown>;
  onChange?: (e: IChangeEvent<Record<string, unknown>>) => void;
  onSubmit?: (e: IChangeEvent<Record<string, unknown>>) => void;
  liveValidate?: boolean;
  children?: React.ReactNode;
}>;

export interface SchemaFormProps {
  /** JSON-Schema object from App.spec.schema (already parsed). */
  schema: unknown;
  /** Current values. */
  formData: Record<string, unknown>;
  /** Called on every change with the latest form data. */
  onChange: (data: Record<string, unknown>) => void;
  /** Optional submit handler — mostly unused because we drive submission from the dialog footer. */
  onSubmit?: (data: Record<string, unknown>) => void;
  /** Optional UI schema overrides. */
  uiSchema?: UiSchema;
  /** Controls the visibility of the built-in submit button. Defaults to hidden. */
  showSubmit?: boolean;
}

// Minimal Shadcn-ish class overrides. RJSF's default classes (`.form-control`,
// `.rjsf` etc.) are DOM-neutral so we map them onto our Tailwind tokens.
const uiClassOverrides: UiSchema = {
  'ui:submitButtonOptions': {
    norender: true,
  },
};

export function SchemaForm({
  schema,
  formData,
  onChange,
  onSubmit,
  uiSchema,
  showSubmit = false,
}: SchemaFormProps) {
  const merged = useMemo<UiSchema>(
    () => ({
      ...(showSubmit ? {} : uiClassOverrides),
      ...(uiSchema ?? {}),
    }),
    [uiSchema, showSubmit]
  );

  const handleChange = (e: IChangeEvent<Record<string, unknown>>) => {
    onChange(e.formData ?? {});
  };

  const handleSubmit = (e: IChangeEvent<Record<string, unknown>>) => {
    onSubmit?.(e.formData ?? {});
  };

  return (
    <div className='novanas-rjsf text-sm'>
      <RjsfForm
        schema={schema ?? {}}
        uiSchema={merged}
        validator={validator}
        formData={formData}
        onChange={handleChange}
        onSubmit={handleSubmit}
        liveValidate
      >
        {showSubmit ? <Button type='submit'>Submit</Button> : <></>}
      </RjsfForm>
    </div>
  );
}
