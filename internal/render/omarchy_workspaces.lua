-- hyprmoncfg owns the global workspace plan. Omarchy's monitor-local
-- handlers must stand down alongside the rules below. Plain Hyprland is unchanged.
if type(o) == "table" and type(o.workspaces) == "table" and type(o.workspaces.reconcile) == "function" and type(o.rebind) == "function" then
  -- Omarchy's event callbacks call this through the module table, including on
  -- startup, reload and hotplug. Disable its reassignment before those run.
  o.workspaces.enabled = false
  o.workspaces.reconcile = function() end

  local function action(expression, fn)
    o.bind_expressions[fn] = expression
    return fn
  end

  function o.workspaces.focus(workspace)
    return action("o.workspaces.focus(" .. string.format("%q", tostring(workspace)) .. ")", function()
      hl.dispatch(hl.dsp.focus({ workspace = tostring(workspace) }))
    end)
  end

  function o.workspaces.move_window(workspace, follow)
    return action("o.workspaces.move_window(" .. workspace .. ", " .. tostring(follow ~= false) .. ")", function()
      hl.dispatch(hl.dsp.window.move({ workspace = tostring(workspace), follow = follow ~= false }))
    end)
  end

  function o.workspaces.move_to_monitor(direction)
    return action("o.workspaces.move_to_monitor(" .. string.format("%q", direction) .. ")", function()
      hl.dispatch(hl.dsp.workspace.move({ monitor = direction }))
    end)
  end

  -- Replace Omarchy's monitor-local number bindings with global destinations.
  for workspace = 1, 10 do
    local key = "code:" .. tostring(workspace + 9)
    o.rebind("SUPER + " .. key, "Switch to workspace " .. workspace, o.workspaces.focus(workspace))
    o.rebind("SUPER + SHIFT + " .. key, "Move window to workspace " .. workspace, o.workspaces.move_window(workspace))
    o.rebind("SUPER + SHIFT + ALT + " .. key, "Move window silently to workspace " .. workspace, o.workspaces.move_window(workspace, false))
  end

  o.rebind("SUPER + TAB", "Next workspace", o.workspaces.focus("e+1"))
  o.rebind("SUPER + SHIFT + TAB", "Previous workspace", o.workspaces.focus("e-1"))
  o.rebind("SUPER + CTRL + TAB", "Former workspace", o.workspaces.focus("previous"))
  o.rebind("SUPER + mouse_down", "Scroll active workspace forward", o.workspaces.focus("e+1"))
  o.rebind("SUPER + mouse_up", "Scroll active workspace backward", o.workspaces.focus("e-1"))

  for key, direction in pairs({ LEFT = "l", RIGHT = "r", UP = "u", DOWN = "d" }) do
    o.rebind("SUPER + SHIFT + ALT + " .. key, "Move workspace to " .. key:lower() .. " monitor", o.workspaces.move_to_monitor(direction))
  end
end
