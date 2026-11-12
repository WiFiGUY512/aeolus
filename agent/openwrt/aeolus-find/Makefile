# The finder as an OpenWrt package (0151): what an image carries so that an
# AP finds an Aeolus manager by its name and joins it, with no one typing
# the manager's installer at it. It holds the files under agent/find/files,
# three small scripts, and none of the agent: the manager found hands out
# its own (0083).
#
# Build it in an OpenWrt buildroot or SDK with this directory as a feed:
#
#	echo "src-link aeolus /path/to/aeolus/agent/openwrt" >> feeds.conf
#	./scripts/feeds update aeolus && ./scripts/feeds install aeolus-find
#	make package/aeolus-find/compile
#
# Or put the three files in an image as they are, at their places.

include $(TOPDIR)/rules.mk

PKG_NAME:=aeolus-find
AEOLUS_DIR:=$(abspath $(CURDIR)/../../..)
PKG_VERSION:=$(shell git -C $(AEOLUS_DIR) describe --tags --match 'v[0-9]*' 2>/dev/null | \
	sed -e 's/^v//' -e 's/-\([0-9]*\)-g[0-9a-f]*$$/_p\1/' | grep . || echo 0.0.0)
PKG_RELEASE:=1

include $(INCLUDE_DIR)/package.mk

define Package/aeolus-find
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Finds an Aeolus manager by its name and joins this AP to it
  PKGARCH:=all
  DEPENDS:=+ucode +ucode-mod-socket
endef

define Package/aeolus-find/description
 While the AP has no Aeolus manager, looks for the name aeolus in the
 domains the network's DHCP gave, and for aeolus.local by multicast DNS.
 What answers from a private address is asked for the manager's installer,
 which installs that manager's agent and joins the AP; it then waits in
 the manager's Landing Zone for a person to adopt it. It does nothing once
 the AP has a manager. /etc/init.d/aeolus-find disable turns it off.
endef

define Build/Compile
endef

# Modes are set here, not taken from the checkout, as the agent's are.
define Package/aeolus-find/install
	$(CP) $(AEOLUS_DIR)/agent/find/files/. $(1)/
	find $(1) -type d -exec chmod 0755 {} +
	find $(1) -type f -exec chmod 0755 {} +
endef

$(eval $(call BuildPackage,aeolus-find))
