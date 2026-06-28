---
date: 2025-01-01T06:00:00Z
title: "CLI and Git integration - Markdown Ninja"
type: "page"
tags: []
authors: ["Markdown Ninja"]
url: "/cli"
---

## Overview

Web interfaces change and evolve over time. While gradual improvement is a nice concept, it too often gets in the way of getting things done and prevents power users from becoming masters of their tools. Digital tools end up owning the artists instead of the other way around.

Would a chef tolerate a knife that would automagically change its shape and weight every night?

The great thing about the command line is that it never changes. The interface is so simple that it doesn't need to be tweaked every week or so. That's real software sustainability.

In 20 years, you will still use the same command line to publish your posts on Markdown Ninja.


## Getting Started

```
$ ls
markdown_ninja.yml
assets/
    image.jpg
blog/
    hello_world.md
pages/
    about.md
```

```bash
$ cat markdown_ninja.yml
```
```yml
# the domain of your website
site: "example.markdown.club"

# The folders where the mdninja CLI can find pages and posts
pages:
  - blog
  - pages
```

```bash
$ cat hello_world.md
```

```markdown
---
date: 2025-01-01T06:00:00Z
title: "Hello World"
type: "post"
tags: ["life"]
authors: ["Markdown Ninja"]
url: "/blog/hello-world"
# draft: true
# To send the post by email to subscribers
# newsletter: true
---

Hello, World.

This is my first blog post with Markdown Ninja and I really love it!

![An image](/assets/image.jpg)

```

Publish with the following command:

```bash
$ docker run -i --rm -e MARKDOWN_NINJA_API_KEY=[YOUR_API_KEY] -v `pwd`:/mdninja ghcr.io/skerkour/markdown-ninja publish
```

## markdown_ninja.yml reference

Coming soon.

## Frontmatter reference

Coming soon.


## GitHub Actions

Create a secret with your Markdown Ninja API Key: `MARKDOWN_NINJA_API_KEY`

```bash
$ cat .github/workflows/publish_website.yml
```

```yml
# publish the content of the website on every commit on the main branch
name: publish_website

on:
  push:
    branches:
      - main

jobs:
  publish_website:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: publish
        env:
          MARKDOWN_NINJA_API_KEY: ${{ secrets.MARKDOWN_NINJA_API_KEY }}
        # You can choose a working directory with the option below
        # working-directory: website
        run: |
          docker run -i --rm -e MARKDOWN_NINJA_API_KEY -v `pwd`:/mdninja ghcr.io/skerkour/markdown-ninja publish
```
