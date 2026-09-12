# Vendored: Shoelace 2.20.1

Source: https://github.com/shoelace-style/shoelace (MIT, see `LICENSE.md`).

This is **not** the full library. Shoelace's `cdn/` build is pre-split
by esbuild into one file per component plus ~2900 shared chunk files
(the whole thing is ~4MB) — there's no single-file bundle to vendor.
Instead, this directory holds only the transitive import closure of
the components actually used (`<sl-button>`, `<sl-badge>`, ...): each
component's own module plus every `chunks/chunk.*.js` it (recursively)
imports. Chunk filenames are content hashes assigned by Shoelace's own
build, not by us.

`themes/light.css` and `themes/dark.css` are unmodified; `dark.css`
scopes its overrides under `.sl-theme-dark`, which is why `app.js`
toggles that class on `<html>` alongside the existing `data-theme`
attribute (see the Theme section of `app.js`).

## Updating, or adding another component

There's no build step here, so this is a manual, occasional step, not
part of any script in this repo:

1. Pick the new version and component(s), e.g. `sl-dialog@2.x.y`.
2. Starting from `https://cdn.jsdelivr.net/npm/@shoelace-style/shoelace@<version>/cdn/components/<name>/<name>.js`,
   follow every relative `import`/`from` path, recursively, resolving
   `../../chunks/chunk.XXXX.js`-style paths the same way a browser
   would. Fetch each file reached this way exactly once.
3. Write each fetched file to the same relative path under this
   directory (`components/<name>/<name>.js`, `chunks/chunk.XXXX.js`,
   ...) — the directory structure must mirror Shoelace's own, since
   the files import each other by relative path.
4. Re-fetch `themes/light.css` / `themes/dark.css` for the new version
   too, and update the version number in this file and in `LICENSE.md`
   if it changed upstream.
5. `import` the new component's module from `app.js` the same way as
   the existing two.

No icons, fonts, or other assets are vendored — neither `<sl-button>`
nor `<sl-badge>` needs them. A component that does (e.g. `<sl-icon>`,
`<sl-alert>` with icons) would need `cdn/assets/icons/*.svg` vendored
too, and `setBasePath()` called from `app.js` pointing at this
directory.
