import {
  Activity,
  Globe,
  Mail,
  Network,
  LayoutDashboard,
  Server,
  Settings,
  type LucideIcon,
} from "lucide-react";

export interface NavItem {
  title: string;
  href: string;
  icon: LucideIcon;
}

/** The main sections of the panel, in sidebar order. */
export const NAV: NavItem[] = [
  { title: "Overview", href: "/", icon: LayoutDashboard },
  { title: "Websites", href: "/websites", icon: Globe },
  { title: "Domains", href: "/domains", icon: Network },
  { title: "Mail", href: "/mail", icon: Mail },
  { title: "Server", href: "/server", icon: Server },
  { title: "Activity", href: "/activity", icon: Activity },
  { title: "Settings", href: "/settings", icon: Settings },
];
