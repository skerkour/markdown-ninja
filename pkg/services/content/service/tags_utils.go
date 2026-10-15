package service

import (
	"context"
	"strings"
	"time"

	"markdown.ninja/pkg/services/content"
	"markdown.ninja/stdx/db"
	"markdown.ninja/stdx/guid"
)

type tagsDiff struct {
	TagsToCreate             []string
	TagPageRelationsToCreate []content.Tag
	TagPageRelationsToRemove []content.Tag
}

func (service *ContentService) diffTags(currentTags []content.Tag, siteTags []content.Tag, newTags []string) (diff tagsDiff, err error) {
	diff = tagsDiff{
		TagsToCreate:             []string{},
		TagPageRelationsToRemove: []content.Tag{},
		TagPageRelationsToCreate: []content.Tag{},
	}

	currentTagsMap := make(map[string]content.Tag)
	for _, tag := range currentTags {
		currentTagsMap[tag.Name] = tag
	}

	siteTagsMap := make(map[string]content.Tag)
	for _, tag := range siteTags {
		siteTagsMap[tag.Name] = tag
	}

	newTagsSet := make(map[string]bool)
	for _, tag := range newTags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		newTagsSet[tag] = true
	}

	for _, currentTag := range currentTags {
		if isInNewtags := newTagsSet[currentTag.Name]; !isInNewtags {
			diff.TagPageRelationsToRemove = append(diff.TagPageRelationsToRemove, currentTag)
		}
	}

	for tag := range newTagsSet {
		if _, alreadyAssociated := currentTagsMap[tag]; !alreadyAssociated {
			existingTag, ok := siteTagsMap[tag]
			if ok {
				diff.TagPageRelationsToCreate = append(diff.TagPageRelationsToCreate, existingTag)
			} else {
				diff.TagsToCreate = append(diff.TagsToCreate, tag)
			}
		}
	}

	for _, newTag := range diff.TagsToCreate {
		err = service.validateTagName(newTag)
		if err != nil {
			return
		}
	}

	return
}

func (service *ContentService) associateTagsToPage(ctx context.Context, db db.Queryer, page content.Page, diff tagsDiff) (err error) {
	now := time.Now().UTC()

	for _, tagToCreate := range diff.TagsToCreate {
		tag := content.Tag{
			ID:          guid.NewTimeBased(),
			CreatedAt:   now,
			UpdatedAt:   now,
			Name:        tagToCreate,
			Description: "",
			WebsiteID:   page.WebsiteID,
		}
		err = service.repo.CreateTag(ctx, db, tag)
		if err != nil {
			return err
		}
	}

	for _, relationToCreate := range diff.TagPageRelationsToCreate {
		relation := content.TagPageRelation{
			PageID: page.ID,
			TagID:  relationToCreate.ID,
		}
		err = service.repo.CreateTagPageRelation(ctx, db, relation)
		if err != nil {
			return err
		}
	}

	for _, relationToDelete := range diff.TagPageRelationsToRemove {
		err = service.repo.DeleteTagPageRelation(ctx, db, page.ID, relationToDelete.ID)
		if err != nil {
			return err
		}
	}

	return nil
}
