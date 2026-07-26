export function channelSelectionId(channel: any): string {
  return channel?.serviceId
    ? `service:${channel.type}:${channel.channel}:${channel.serviceId}`
    : (channel?.channel || channel?.id || '')
}

export function channelFromEPG(channel: {
  channel: string
  type: string
  serviceId: number
  name: string
}): any {
  return {
    channel: channel.channel,
    type: channel.type,
    serviceId: channel.serviceId,
    name: channel.name,
    displayName: channel.name,
    isService: true,
    services: [{ id: channel.serviceId, serviceId: channel.serviceId, name: channel.name }],
  }
}

export function expandChannelServices(allChannels: any[]): any[] {
  const result: any[] = []
  for (const channel of allChannels) {
    if (!channel.services?.length) continue
    if (channel.services.length > 1) {
      for (const service of channel.services) {
        result.push({
          ...channel,
          serviceId: service.id,
          serviceName: service.name,
          displayName: service.name,
          originalChannel: channel.channel,
          isService: true
        })
      }
    } else {
      result.push({
        ...channel,
        displayName: channel.services[0]?.name || channel.name,
        serviceId: channel.services[0]?.id,
        isService: false
      })
    }
  }
  return result
}

export function channelMatchesSelection(channel: any, selectionId: string): boolean {
  if (!channel || !selectionId) return false
  if (!selectionId.startsWith('service:')) {
    return channel.channel === selectionId || channel.id === selectionId
  }
  const parts = selectionId.slice('service:'.length).split(':')
  if (parts.length !== 3) return false
  const [type, physicalChannel, rawServiceId] = parts
  if (channel.type !== type || channel.channel !== physicalChannel) return false
  const requestedServiceId = Number(rawServiceId)
  // Expanded service entries must only match their own service. Falling back to
  // the parent service list would make every subchannel match the first entry.
  if (channel.serviceId !== undefined && channel.serviceId !== null) {
    return Number(channel.serviceId) === requestedServiceId
  }
  return channel.services?.some((service: any) =>
    Number(service.id) === requestedServiceId || Number(service.serviceId) === requestedServiceId
  ) ?? false
}

export function resolveActiveChannel(allChannels: any[], selectionId: string): any | null {
  return expandChannelServices(allChannels).find(channel => channelMatchesSelection(channel, selectionId)) ?? null
}
