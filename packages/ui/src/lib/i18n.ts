/**
 * Lingui i18n bootstrap.
 *
 * All routes and user-visible components use `<Trans>` / `i18n._()` from
 * `@lingui/react`. English is currently the only locale; we activate an
 * identity catalog (no translations loaded) so messages render as their
 * source strings untouched. When real translations land, replace this with
 * the output of `lingui compile` and call `i18n.load(locale, messages)`
 * per locale.
 */
import { i18n } from '@lingui/core';

// Minimal identity catalog. Lingui will render the source copy for any message
// not present here. When real translations land, replace this with the output of
// `lingui compile` and call `i18n.load(locale, messages)` per locale.
const messages: Record<string, string> = {};

i18n.load('en', messages);
i18n.activate('en');

export { i18n };
