---
title: "Unrolling a globe into a flat map in the vertex shader"
date: 2026-08-10
description: "How ImmersiveMap morphs a sphere into a Mercator plane in the vertex shader, and why a straight lerp between the two looks wrong."
tags: [metal, shaders, swift]
---

The globe in ImmersiveMap is there because it looks good. That is the whole
reason. It is not a more honest projection and it does not show anything better:
at low zoom you get a planet on screen instead of a rectangle.

Which is exactly why the transition carries all the weight. A globe that snaps
into a flat map is a broken effect, and the effect is the entire point of having
a globe. So the shape is one value in `0...1`, recomputed every frame from the
zoom, and the vertex shader morphs the geometry with it.

Internally there are still two render surfaces, spherical and flat, running
different layers: extruded buildings and shadows on the flat path, the starfield
and the day/night terminator on the spherical one. The changeover between them
happens once, at the end of the window, and a good deal of what follows exists to
put it on a frame where nobody can see it.

This post is about that value, and about the two things that had to change before
the transition stopped looking wrong.

## Why not just keep the globe

The obvious question, given that the globe is the part people actually like: why
unroll it at all, instead of staying on the sphere the whole way down?

Because of the floats. A sphere puts every vertex at planet radius from the
origin, and the shader computes in 32-bit floats, whose precision is relative:
the further a coordinate sits from zero, the wider the gap between the numbers
you can actually represent. At city zoom you are asking for street-level detail
on top of a radius that dwarfs it, and the bits are not there. The geometry
starts to shimmer and swim as the camera moves, and no care taken in the
projection recovers it, because the precision is already gone before the
projection runs.

The flat path sidesteps this by keeping the numbers small. Positions are computed
relative to the camera center (`panX`, `panMercatorY` in the shader) rather than
to the center of a planet, and the world size is normalised per zoom level
(`pow(2, floor(zoom))`), so zooming deeper does not grow the coordinates that the
shader has to resolve.

So the globe is for looking at, the plane is for working on, and the map has to
get from one to the other without anybody noticing the handover.

## Where the value comes from

The camera gives a zoom. The resolver turns it into a transition:

```swift
let from = settings.automaticTransitionStartZoom
let latitude = ImmersiveMapProjection.latitude(fromNormalizedWorldY: cameraState.centerWorldMercator.y)
let latitudeSpanExtension = log2(1.0 / max(cos(latitude), 0.01))
let span = max(.leastNonzeroMagnitude, settings.automaticTransitionSpan + latitudeSpanExtension)
return Float(max(0.0, min(1.0, (cameraState.zoom - from) / span)))
```

By default the sphere starts unrolling at zoom 6 and takes one zoom level. The
interesting part is `latitudeSpanExtension`, and it is the first thing that is
not obvious about morphing to Mercator.

A Mercator plane stretches by `1/cos(latitude)`. The morph target is not a fixed
sheet: over the transition it inflates from `cos(latitude)` to the full Mercator
size, because that is what a Mercator projection of that place actually is.

```metal
static inline float globeTransitionMapSize(constant Globe& globe,
                                           float panLatitude) {
    float distortion = cos(panLatitude);
    float mapSizeScale = mix(distortion, 1.0, globe.transition);
    return 2.0 * M_PI_F * globe.radius * mapSizeScale;
}
```

So the transition plays back `log2(1/cos(latitude))` levels of visible inflation
on top of the zoom the user is doing. At the equator that is nothing. At Tokyo's
35.7 degrees it is about 0.3 zoom levels, and at 60 degrees it is a full one:
the same one-level window would run twice as fast in Stockholm as in Nairobi.
Widening the window by exactly that amount is what makes the speed of the unroll
independent of where you are on the planet.

Both a globe render state and a flat render state are produced every frame
regardless of the value. What changes is how the shaders blend them and which
layers the frame graph runs.

## The straight lerp, and why it fails

With the transition in hand, the vertex shader has a sphere position and a plane
position for every vertex, and the obvious thing is to interpolate between them:

```metal
float4 position = mix(spherePositionTranslated, flatPosition, transition);
```

This is correct. Both endpoints are exact, nothing is distorted, and it looks
bad.

The reason is that the two positions of a vertex are not the same distance
apart everywhere. A point near the view center barely moves: it is roughly
where it will end up already. A point near the far corner of the Mercator sheet
travels an enormous distance, because Mercator sends the poles to infinity and
the sheet is being inflated at the same time. Under a uniform lerp every vertex
covers its own distance in the same time, so the far corners move fastest, and
they arrive at the same moment as everything else. The planet does not unroll.
It inflates, all at once, like a balloon with the corners leading.

What a sheet of paper does is the opposite. The part in front of you settles
first, and the curl travels outward.

## The wave

The fix is to give each vertex its own phase, lagging the global one in
proportion to how far that vertex is from the view center:

```metal
static inline float globeTransitionLocalPhase(float transition, float frontDot) {
    const float spread = 0.6;
    float lagWeight = acos(clamp(frontDot, -1.0, 1.0)) / M_PI_F;
    return clamp((transition - lagWeight * spread) / (1.0 - spread), 0.0, 1.0);
}
```

`frontDot` is the cosine of the angle between the vertex and the view direction
after the planet is rotated, so `lagWeight` runs from 0 at the center of the
visible face to 1 at the antipode. `spread` is how much of the transition is
spent on the lag: with 0.6, the area facing the camera is fully unrolled by the
time the global value reaches 0.4, while the antipodal point only starts moving
at 0.6. The division by `1.0 - spread` rescales what is left so every vertex
still lands exactly at 1.

Both extremes are unchanged. At `transition = 0` every local phase is 0 and the
surface is the sphere; at 1 every phase is 1 and it is the plane. The wave lives
entirely in the middle, which is the only place it is visible at all.

The whole use of it in the vertex shader is two lines:

```metal
float3 rotatedSphereDirection = normalize((float4(spherePosition, 0.0) * rotation).xyz);
float localTransition = globeTransitionLocalPhase(transition, rotatedSphereDirection.z);
float4 position = mix(spherePositionTranslated, flatPosition, localTransition);
```

The function has a mirror in Swift, `GeoScreenProjectionMath.transitionLocalPhase`,
because anything the CPU places on the surface has to ride the same wave: marker
anchors, avatars, 3D models, the tile atlas planner. If the mirror drifts from the
shader, markers slide off their tiles halfway through the morph, which is exactly
the kind of bug that only shows up in motion.

## The last ten percent

This is where the changeover from the introduction comes back. Once the
transition reaches 1 the frame graph runs the flat path, with extruded buildings
and shadows that the globe path does not draw. A switch between two code paths
between one frame and the next is a chance for a visible pop.

The trick is to finish the geometry early:

```swift
private static let geometryCompletionPhase: Float = 0.9

private static func geometryTransition(_ transition: Float) -> Float {
    min(1.0, transition / geometryCompletionPhase)
}
```

The shader gets a value that reaches 1 at 90 percent of the window, so for the
last tenth the globe path is drawing a plane that is already finished. When the
surface switch happens at 1, it happens between two geometrically identical
frames. Meanwhile the semantic transition stays continuous all the way to 1, so
fades, fog and layer selection still have somewhere to land.

## Markers have no depth buffer

Tiles are drawn with depth testing, so the back of the globe is hidden for free.
SwiftUI markers and avatars are an overlay: they have no depth, and mid-morph
they will happily render points from the far side of the planet.

The visibility test therefore has to use the same local phase, not the global
transition. A point sitting on the still-spherical part has local phase 0 and
must pass the strict spherical horizon test, or the back side leaks through.

There is a second gate on top, and it comes from a failure that only appears at
strong tilt. A marker whose local phase has already unfurled its position toward
the plane can be dragged across the viewport on its way to its flat spot: it hangs
over empty ocean for a few frames, because the tiles under it are far coverage
and are not drawn at all. Tiles never show that transit; a marker does. So a
point far past the spherical horizon stays hidden even when its local phase says
otherwise, with a margin (`unfurlVisibilityMarginRadians`, 0.5 radians) for the
points the unroll legitimately brings into frame when the camera is tilted.

Both fades are soft bands rather than hard steps. A marker that pops in at the
horizon draws the eye immediately, which is why the band is worth the extra
arithmetic.

## What you can change

Nothing above needs configuring. A bare `ImmersiveMapView()` already morphs. The
window is tunable:

```swift
ImmersiveMapView()
    .presentationSettings(ImmersiveMapSettings.PresentationSettings(
        automaticTransitionStartZoom: 6.0,
        automaticTransitionSpan: 1.0,
        globeRadiusScale: 0.14))
```

There is no public "force globe" or "force flat" switch: the presentation follows
the camera. The supported way to pin it is to keep the camera on one side of the
window:

```swift
ImmersiveMapView()
    .zoomRange(minimum: 8)     // always flat
```

```swift
ImmersiveMapView()
    .zoomRange(maximum: 5)     // always a globe
```

Gestures, camera commands and flights are all clamped to the same range, so this
holds against the user as well as against the app.

One practical note: a tilted pass through the window is the most demanding thing
the renderer does, and a slow crossing of three to five seconds reads far better
than a fast one. The wave is only visible
if you give it time to travel.

The engine is MIT licensed and the code is on
[GitHub](https://github.com/artembobkin/ImmersiveMap). The shader in this post is
`Render/Shaders/Globe/GlobeTransitionProjection.h`, and its Swift mirror is
`Render/Projection/GeoScreenProjectionMath.swift`.
