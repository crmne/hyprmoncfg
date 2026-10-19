---
title: 2-in-1s and Tablets
description: Turn the built-in panel with the device, keep the monitors beside it in place, and lock the rotation when you need to.
nav_order: 7
---

## Automatic rotation

On a 2-in-1 laptop or a tablet, hyprmoncfg can turn the built-in panel with the
device. It reads the accelerometer through iio-sensor-proxy, so there is no
script to write and nothing fights over the rotation:

- The panel turns within a moment of the device settling.
- Monitors beside it move to stay touching, the way they do when you change a
  display's resolution.
- The touchscreen and pen turn with the panel.
- Every Hyprland reload keeps the current rotation, because the generated
  config always holds it.

Your saved profile never changes. It keeps describing the layout with the
device upright, and the rotation is applied on top while hyprmoncfgd runs.

## Setting it up

1. Install iio-sensor-proxy, which provides the accelerometer:

   ```bash
   sudo pacman -S iio-sensor-proxy
   ```

2. Make sure your user can read the tablet-mode switch, so a convertible only
   turns when folded into a tablet. Omarchy users are in the `input` group
   already; check with `id`, and add yourself with
   `sudo usermod -aG input $USER` if not.
3. In the TUI or the Omarchy panel, select the built-in panel and set
   **Rotation** to **Auto**. Save the profile.
4. Remove any script of your own that rotates the screen, such as one built on
   `monitor-sensor` or `iio-hyprland`. Two programs rotating the same panel
   will undo each other.

**Auto** appears only for a built-in panel, and only when hyprmoncfgd reports
an orientation sensor. Restart hyprmoncfgd after installing iio-sensor-proxy.

## Upright

The rotation the panel has when you choose **Auto** is its upright rotation.
For most devices that is **Normal**. Some tablets have a panel mounted in
portrait: set the rotation that looks right with the device upright first,
then choose **Auto**. The inspector then shows, for example,
**Auto, upright 270°**, and the sensor's turns add to that.

### Calibrate the sensor's upright position

If Auto adds a quarter turn even though you selected the correct fixed rotation,
hold the device upright relative to its keyboard (or the way you normally use a
tablet), then run:

```bash
hyprmoncfg rotation calibrate
```

This records the sensor reading as upright and unlocks rotation. It leaves saved
profiles unchanged and persists across daemon restarts. Set the panel's correct
fixed rotation, such as 270°, before choosing Auto. Direction reversal changes
which way turns go; calibration changes where they start. Recalibrate after
changing the system sensor mount matrix. Calibration requires a current daemon
and refuses to run during a preview or while the sensor reports the device flat.

### Preview behavior

A preview applies the orientation at its start and holds it while you decide.
Auto rotation resumes after **Keep**, including when keeping a temporary layout
without saving a profile. This prevents sensor changes from moving the
confirmation controls or racing rollback.

## Tablet mode

A convertible with a tablet-mode switch turns its panel only while folded
into a tablet, as GNOME does, so a laptop tilted on your lap stays put. Folded
back into a laptop, the panel goes upright. A device with no such switch, such
as a tablet or a detachable, always follows the sensor. Lying flat keeps the
last rotation.

## Rotation lock

Lock the rotation to keep the panel as it is, for example to read lying down:

```bash
hyprmoncfg rotation lock     # hold the current rotation
hyprmoncfg rotation unlock   # follow the device again
hyprmoncfg rotation toggle   # switch between the two
hyprmoncfg rotation          # show the current state
```

Bind `hyprmoncfg rotation toggle` to a key for a one-press lock. On Omarchy,
add it to `~/.config/hypr/bindings.lua`:

```lua
o.bind("SUPER + O", "Rotation lock", "hyprmoncfg rotation toggle")
```

Elsewhere, bind it the way your config runs any other command.

In the TUI, press `L`. In the Omarchy panel, use the lock beside **Auto**. The
lock is remembered across restarts. Unlocking turns the panel to the way the
device is held now.

## If it turns the wrong way

If the panel turns the opposite way to the device, reverse the direction:

```bash
hyprmoncfg rotation direction reversed
```

Undo it with `hyprmoncfg rotation direction normal`. The setting belongs to
the device, not to a profile, and survives restarts. If the panel is the
right way round only when the device is turned, set its upright rotation
instead (see [Upright](#upright)). If the sensor reports a non-normal orientation while you hold the device
upright, calibrate it as described above.

Both come from the device's accelerometer mount matrix in the system hardware
database, which is missing or wrong for some models, and GNOME and KDE turn
those wrong too. To fix it for every program at once, add an
`ACCEL_MOUNT_MATRIX` entry for your device to
`/etc/udev/hwdb.d/61-sensor-local.hwdb`, as the
[iio-sensor-proxy documentation](https://gitlab.freedesktop.org/hadess/iio-sensor-proxy/)
explains, and consider sending it upstream to systemd. Then set the direction
back to normal.

## Limits

- Fixed rotations of a connected, enabled built-in panel also keep touch and
  pen aligned, including after disabling Auto or restarting Hyprland.
- Touch and pen rotation use Hyprland's global `input:touchdevice:transform`
  and `input:tablet:transform`. An external drawing tablet turns with the
  panel too.
- Only built-in panels rotate. External monitors don't turn with the device.
- Rotation needs hyprmoncfgd. Applying a profile from the TUI or CLI without
  the daemon uses the upright layout.
