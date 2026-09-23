---
title: "The engine in motion"
date: 2026-08-01
description: "A short look at what ImmersiveMap renders: the globe morphing into a flat map, vector-tile labels, avatar markers and the starfield."
tags: [overview, metal]
video: iMwP0WKyyzQ
---

This is the overview clip from the home page: a first look at what the engine
draws before any of the how-to screencasts get into the details.

What is on screen:

- the globe morphing into a flat map as the camera comes down,
- vector-tile labels placed and collision-checked on the GPU,
- avatar markers tracking positions on the surface,
- the starfield behind the globe.

Everything renders natively in Metal. There is no web view anywhere in the
stack, and the tiles come from the built-in provider, so the same thing runs
against a free key from your account and nothing else to configure.
