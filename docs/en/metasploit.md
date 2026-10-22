# Metasploit Framework documentation

The `metasploit` parser imports the current official documentation from
[docs.metasploit.com](https://docs.metasploit.com/) under the fixed slug
`metasploit`. It reads the sitemap and the rendered documentation pages;
it does not execute Metasploit or import the entire module source tree.

```bash
make parsers
./scripts/run-parser.sh metasploit
./bin/marvindocs index build
./bin/marvindocs search msfconsole --doc metasploit
./bin/marvindocs get metasploit index
```

`make update` includes this parser once its binary has been built.
Indexing is incremental; existing embeddings for other documentation
are preserved. Embeddings for the new documentation can be generated with
`./bin/marvindocs index embed --doc metasploit` using your configured backend.

The output in `data/public/docs/metasploit/` contains `meta.json`,
`index.json`, `db.zip` and `.last-modified`. The runner also writes `.parser`
to protect the imported pages from DevDocs synchronization.

The parser keeps the main content, code blocks, tables and heading anchors,
removes site navigation and footers, and rewrites links to imported pages
as relative local links. Images and links to pages outside the imported
set remain online. Any failed download or missing content stops the import
before writing the output, preserving the previous corpus.

Direct invocation:

```bash
./bin/metasploit-parser --out data/public/docs/metasploit --force
```

`--source` overrides the site root (a compatible mirror must provide
`sitemap.xml` and the same HTML structure). `--version` and `--release`
only set metadata labels; they do not select a historical release.
The default corpus is the current unversioned website.

Source: `parsers/metasploit/`. Tests: `make parsers-test`.
