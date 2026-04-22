/**
 * Lingui i18n bootstrap.
 *
 * Wave 12 (B1-UI-Batch): Dashboard, Storage (pools + datasets), and Chrome
 * (sidebar/topbar) are migrated to use <Trans>/t`` from @lingui/react.
 *
 * English is currently the only locale. We activate an identity catalog (no
 * translations loaded) so `<Trans>` emits the source string untouched. This is
 * intentional — translation catalogs can be generated later via `lingui extract`
 * without any code changes.
 *
 * Other routes still have inline English. See `// TODO(i18n-wave-12)` markers.
 */
import { i18n } from '@lingui/core';

// Minimal identity catalog. Lingui will render the source copy for any message
// not present here. When real translations land, replace this with the output of
// `lingui compile` and call `i18n.load(locale, messages)` per locale.
const messages: Record<string, string> = {};

i18n.load('en', messages);
i18n.activate('en');

export { i18n };
