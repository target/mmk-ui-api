import { differenceInMinutes } from 'date-fns'
import { Site, SiteAttributes } from '../models'

const getRunnable = async (): Promise<Site[]> => {
  const whereQuery: Partial<SiteAttributes> = {
    active: true,
  }
  const sites = await Site.query().where(whereQuery)
  const now = new Date()
  const res: Site[] = []
  for (let i = 0; i < sites.length; i += 1) {
    const site = sites[i]
    const diff = differenceInMinutes(now, site.last_run)
    if (diff > site.run_every_minutes || isNaN(diff)) {
      res.push(site)
    }
  }
  return res
}

const view = async (id: string): Promise<Site> =>
  Site.query().findById(id).skipUndefined().throwIfNotFound()

const create = async (attrs: Partial<SiteAttributes>): Promise<Site> =>
  Site.query().insert(attrs)

const update = async (
  id: string,
  site: Partial<SiteAttributes>
): Promise<Site> => {
  const updated = await Site.query().patchAndFetchById(id, site)
  return updated
}

const destroy = async (id: string): Promise<number> =>
  Site.query().deleteById(id)

export default {
  getRunnable,
  view,
  update,
  create,
  destroy,
}
