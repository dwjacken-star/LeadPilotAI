# Menu, mailbox localization and AI options repair

Management route metadata now retains translation keys for all menu entries, including the department workspace and secondary tabs. Secondary tabs translate when rendered, so changing the language updates already-open menus without discarding form state.

Mailbox login uses a six-language select control, and the sample conversation labels are localized. Original Roundcube dictionaries are supplemented only where upstream translations fall back to English: 60 Chinese, 3 Japanese, 23 German, 2 Turkish and 237 Vietnamese labels/messages. Template variables are checked before installing overlays. System tab names are retranslated on restore; message subjects and user folder names remain user content.

The native PHP proxy preserves empty JSON objects instead of serializing them as arrays. An authenticated unlinked mailbox may read credential-free platform/model metadata. It remains unable to generate content or access CRM until an explicit business permission binding is configured. Error feedback explains this condition.

Production inspection found no mailbox business bindings and only the admin management account. The user was asked whether sales@best-maple.com should receive AI-writing-only access; no administrator binding is installed implicitly. Browser visual verification is separate from authenticated HTTP, source compilation and proxy checks.
