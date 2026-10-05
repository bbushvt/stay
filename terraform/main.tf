terraform {
  required_version = ">= 1.0"
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.0"
    }
  }
}

variable "agent_id" {
  type        = string
  description = "The ID of a Coder agent."
}

variable "stay_version" {
  type        = string
  default     = "latest"
  description = "STAY release to install: \"latest\" or a version such as \"0.1.0\" (no leading v)."
  validation {
    condition     = var.stay_version == "latest" || can(regex("^[0-9]+\\.[0-9]+\\.[0-9]+([-+][0-9A-Za-z.-]+)?$", var.stay_version))
    error_message = "stay_version must be \"latest\" or a semantic version like 0.1.0."
  }
}

variable "port" {
  type        = number
  default     = 7681
  description = "Loopback port STAY listens on inside the workspace."
  validation {
    condition     = var.port >= 1024 && var.port <= 65535
    error_message = "port must be between 1024 and 65535."
  }
}

variable "layout" {
  type        = string
  default     = ""
  description = "Optional layout YAML, written to ~/.config/stay/layout.yaml on every workspace start. Empty keeps whatever file is there (or the built-in layout)."
}

variable "repo" {
  type        = string
  default     = "bbushvt/stay"
  description = "GitHub owner/name that publishes STAY releases."
  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$", var.repo))
    error_message = "repo must look like owner/name."
  }
}

variable "install_dir" {
  type        = string
  default     = "$HOME/.local/bin"
  description = "Where the stay binary is installed. May reference $HOME."
  validation {
    condition     = can(regex("^[A-Za-z0-9_./$-]+$", var.install_dir))
    error_message = "install_dir may only contain letters, digits and _ . / $ -"
  }
}

variable "share" {
  type        = string
  default     = "owner"
  description = "Who can open the app. STAY has no authentication of its own and a terminal is a shell, so keep this \"owner\" unless you are sure."
  validation {
    condition     = contains(["owner", "authenticated", "public"], var.share)
    error_message = "share must be owner, authenticated or public."
  }
}

variable "order" {
  type        = number
  default     = null
  description = "Position of the app button in the workspace UI."
}

variable "group" {
  type        = string
  default     = null
  description = "Group name for the app button."
}

resource "coder_script" "stay" {
  agent_id     = var.agent_id
  display_name = "STAY"
  icon         = "/icon/terminal.svg"
  run_on_start = true
  # Starting is quick, but never hold up login if GitHub is slow.
  start_blocks_login = false

  # Settings are exported ahead of the static script so run.sh needs no
  # Terraform templating (and so no escaping of shell syntax).
  script = join("\n", [
    "export STAY_VERSION=${var.stay_version}",
    "export STAY_PORT=${var.port}",
    "export STAY_REPO=${var.repo}",
    "export STAY_INSTALL_DIR=\"${var.install_dir}\"",
    "export STAY_LAYOUT_B64=${base64encode(var.layout)}",
    file("${path.module}/run.sh"),
  ])
}

resource "coder_app" "stay" {
  agent_id     = var.agent_id
  slug         = "stay"
  display_name = "STAY"
  icon         = "/icon/terminal.svg"
  url          = "http://127.0.0.1:${var.port}"
  # Subdomain apps get their own origin, which the websocket same-origin check
  # and relative asset URLs both rely on.
  subdomain = true
  share     = var.share
  order     = var.order
  group     = var.group

  healthcheck {
    url       = "http://127.0.0.1:${var.port}/"
    interval  = 5
    threshold = 6
  }
}

output "app_url" {
  value       = coder_app.stay.url
  description = "In-workspace URL of STAY."
}
