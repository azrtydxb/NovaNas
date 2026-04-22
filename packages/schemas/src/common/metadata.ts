import { z } from 'zod';

export const LabelsSchema = z.record(z.string(), z.string());
export type Labels = z.infer<typeof LabelsSchema>;

export const AnnotationsSchema = z.record(z.string(), z.string());
export type Annotations = z.infer<typeof AnnotationsSchema>;

/**
 * Kubernetes-like ObjectMeta. Only the fields NovaNas cares about.
 */
export const ObjectMetaSchema = z.object({
  name: z.string().min(1),
  namespace: z.string().optional(),
  labels: LabelsSchema.optional(),
  annotations: AnnotationsSchema.optional(),
  uid: z.string().optional(),
  resourceVersion: z.string().optional(),
  generation: z.number().int().optional(),
  creationTimestamp: z.string().datetime({ offset: true }).optional(),
  deletionTimestamp: z.string().datetime({ offset: true }).optional(),
  finalizers: z.array(z.string()).optional(),
  ownerReferences: z
    .array(
      z.object({
        apiVersion: z.string(),
        kind: z.string(),
        name: z.string(),
        uid: z.string(),
        controller: z.boolean().optional(),
        blockOwnerDeletion: z.boolean().optional(),
      })
    )
    .optional(),
});
export type ObjectMeta = z.infer<typeof ObjectMetaSchema>;
