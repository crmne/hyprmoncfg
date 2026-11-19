-- hyprmoncfg gives each display workspaces of its own, numbered in blocks of
-- ten: the first display 1-10, the next 11-20.
local slots = 10 -- hyprmoncfg:slots
if type(o) == "table" and type(o.workspaces) == "table" and type(o.workspaces.reconcile) == "function" then
  -- Omarchy already does this. It numbers each monitor's block, binds
  -- Super + number, brings workspaces home on hotplug, and its bar shows each
  -- monitor's own. It stays in charge, and this file adds no rules. It only
  -- takes back the number keys, which Omarchy binds by key code, past the
  -- workspaces each display has.
  for slot = slots + 1, 10 do
    local code = "code:" .. tostring(slot + 9)
    for _, mods in ipairs({ "SUPER", "SUPER + SHIFT", "SUPER + SHIFT + ALT" }) do
      hl.unbind(mods .. " + " .. code)
    end
  end
else
  -- Each display's block, by the selector its monitor rule uses.
  local blocks = {} -- hyprmoncfg:blocks

  local function slot_workspace(slot)
    local monitor = hl.get_active_monitor()
    local block = monitor and (blocks["desc:" .. (monitor.description or "")] or blocks[monitor.name])
    if not block then
      return tostring(slot)
    end
    return tostring(block * 10 + slot)
  end

  -- Super + number goes to that workspace on the focused display,
  -- Super + Shift + number moves the focused window there, and
  -- Super + Shift + Alt + number moves it without following. Both the key
  -- symbol and the key code are unbound, so an existing binding of either
  -- does not also fire. Keys past the display's workspaces do nothing.
  for slot = 1, 10 do
    local symbol = tostring(slot % 10)
    local code = "code:" .. tostring(slot + 9)
    for _, mods in ipairs({ "SUPER", "SUPER + SHIFT", "SUPER + SHIFT + ALT" }) do
      hl.unbind(mods .. " + " .. symbol)
      hl.unbind(mods .. " + " .. code)
    end
    if slot <= slots then
      hl.bind("SUPER + " .. code, function()
        hl.dispatch(hl.dsp.focus({ workspace = slot_workspace(slot) }))
      end, { description = "Switch to workspace " .. slot .. " on this display" })
      hl.bind("SUPER + SHIFT + " .. code, function()
        hl.dispatch(hl.dsp.window.move({ workspace = slot_workspace(slot), follow = true }))
      end, { description = "Move window to workspace " .. slot .. " on this display" })
      hl.bind("SUPER + SHIFT + ALT + " .. code, function()
        hl.dispatch(hl.dsp.window.move({ workspace = slot_workspace(slot), follow = false }))
      end, { description = "Move window silently to workspace " .. slot .. " on this display" })
    end
  end

  -- hyprmoncfg:rules
end
